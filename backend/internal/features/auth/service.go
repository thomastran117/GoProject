package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/external/cloudflare"
	"backend/internal/external/email"
	"backend/internal/external/google"
	"backend/internal/external/microsoft"
	"backend/internal/features/device"
	"backend/internal/features/token"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// oauthVerifyTimeout caps the total time spent verifying a single OAuth token,
// including all retry attempts. Applied on top of any caller-supplied deadline.
const oauthVerifyTimeout = 15 * time.Second

type AuthResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	RefreshTTL   time.Duration `json:"-"`
	User         UserData      `json:"user"`
}

type UserData struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// LoginOutcome is returned by Login and OAuth authenticate methods.
// Exactly one of Auth or DeviceChallenge is non-nil.
type LoginOutcome struct {
	Auth            *AuthResponse
	DeviceChallenge *DeviceChallengeResponse
}

// DeviceChallengeResponse is returned when a login attempt comes from an
// unrecognized device. Tokens are withheld until the user verifies via email.
type DeviceChallengeResponse struct {
	Message string `json:"message"`
}

// SignupPendingResponse is returned by Signup to indicate that a verification
// email has been sent and account creation is pending email confirmation.
type SignupPendingResponse struct {
	Message string `json:"message"`
}

// pendingSignup holds the data stored in Redis while awaiting email verification.
type pendingSignup struct {
	Email        string  `json:"email"`
	PasswordHash string  `json:"password_hash"`
	Role         string  `json:"role"`
	SchoolID     *uint64 `json:"school_id,omitempty"`
	RememberMe   bool    `json:"remember_me"`
}

// pendingDeviceAuth holds the data stored in Redis while awaiting device
// verification. It carries enough context to issue tokens once confirmed.
type pendingDeviceAuth struct {
	UserID      uint64 `json:"user_id"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Fingerprint string `json:"fingerprint"`
	DeviceType  string `json:"device_type"`
	UserAgent   string `json:"user_agent"`
	RememberMe  bool   `json:"remember_me"`
}

const pendingSignupTTL    = 24 * time.Hour
const pendingDeviceAuthTTL = 15 * time.Minute

// deviceRepository is the subset of device.Repository methods used by the auth
// service. Defined here as an interface so that tests can substitute a mock.
type deviceRepository interface {
	FindByUserAndFingerprint(userID uint64, fingerprint string) (*device.Device, error)
	Create(userID uint64, fingerprint, deviceType, userAgent string) (*device.Device, error)
	UpdateLastSeen(id uint64) error
}

type Service struct {
	repo               *Repository
	deviceRepo         deviceRepository
	googleVerifier     *google.Client
	msVerifier         *microsoft.Client
	turnstileSecretKey string
	skipTurnstile      bool
	httpClient         *http.Client
	redisClient        *redis.Client
	emailSender        email.Sender // nil in dev when email is not configured
	appURL             string
	schoolExists       func(ctx context.Context, id uint64) (bool, error)
}

func NewService(
	repo *Repository,
	deviceRepo deviceRepository,
	googleClientID, microsoftClientID, turnstileSecretKey string,
	skipTurnstile bool,
	redisClient *redis.Client,
	emailSender email.Sender,
	appURL string,
	schoolExists func(ctx context.Context, id uint64) (bool, error),
) *Service {
	// Shared across all OAuth and captcha requests. The 10-second timeout is a
	// hard cap; per-request context deadlines still take precedence.
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return &Service{
		repo:               repo,
		deviceRepo:         deviceRepo,
		googleVerifier:     google.NewClient(httpClient, googleClientID),
		msVerifier:         microsoft.NewClient(httpClient, microsoftClientID),
		turnstileSecretKey: turnstileSecretKey,
		skipTurnstile:      skipTurnstile,
		httpClient:         httpClient,
		redisClient:        redisClient,
		emailSender:        emailSender,
		appURL:             appURL,
		schoolExists:       schoolExists,
	}
}

// --- public interface ---

func (s *Service) Login(ctx context.Context, addr, password, captcha string, rememberMe bool, clientInfo middleware.ClientInfo) (*LoginOutcome, error) {
	if !s.skipTurnstile {
		if err := cloudflare.VerifyTurnstile(ctx, s.httpClient, s.turnstileSecretKey, captcha); err != nil {
			return nil, err
		}
	}

	user, err := s.repo.FindByEmail(addr)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid email or password",
		}
	}

	if err := s.ComparePassword(user.PasswordHash, password); err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid email or password",
		}
	}

	return s.checkDeviceAndIssueTokens(ctx, user.ID, user.Email, user.Role, rememberMe, clientInfo)
}

func (s *Service) Signup(ctx context.Context, addr, password, captcha, role string, schoolID *uint64, rememberMe bool) (*SignupPendingResponse, error) {
	if !IsValidSignupRole(role) {
		return nil, &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_ROLE",
			Message: "Role must be one of: student, teacher, principal, teaching_assistant",
		}
	}

	if role == RoleTeacher {
		if schoolID == nil {
			return nil, &middleware.APIError{
				Status:  http.StatusBadRequest,
				Code:    "SCHOOL_REQUIRED",
				Message: "Teachers must provide a school ID",
			}
		}
		exists, err := s.schoolExists(ctx, *schoolID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &middleware.APIError{
				Status:  http.StatusBadRequest,
				Code:    "SCHOOL_NOT_FOUND",
				Message: "The specified school does not exist",
			}
		}
	}

	if !s.skipTurnstile {
		if err := cloudflare.VerifyTurnstile(ctx, s.httpClient, s.turnstileSecretKey, captcha); err != nil {
			return nil, err
		}
	}

	existing, err := s.repo.FindByEmail(addr)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusConflict,
			Code:    "USER_EXISTS",
			Message: "An account with this email already exists",
		}
	}

	hash, err := s.HashPassword(password)
	if err != nil {
		return nil, err
	}

	verifyToken := uuid.New().String()
	pending := pendingSignup{
		Email:        addr,
		PasswordHash: hash,
		Role:         role,
		SchoolID:     schoolID,
		RememberMe:   rememberMe,
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return nil, err
	}

	key := pendingSignupKey(verifyToken)
	if err := s.redisClient.Set(ctx, key, data, pendingSignupTTL).Err(); err != nil {
		return nil, err
	}

	if s.emailSender != nil {
		verifyURL := fmt.Sprintf("%s/verify?token=%s", s.appURL, verifyToken)
		body := fmt.Sprintf("Please verify your email address by visiting the link below:\n\n%s\n\nThis link expires in 24 hours.", verifyURL)
		go email.SendWithRetry(context.Background(), s.emailSender, addr, "Verify your email", body)
	}

	return &SignupPendingResponse{Message: "Verification email sent. Please check your inbox."}, nil
}

// VerifyEmail looks up the pending signup by token, creates the user account,
// and returns a full auth response.
func (s *Service) VerifyEmail(ctx context.Context, verifyToken string, clientInfo middleware.ClientInfo) (*LoginOutcome, error) {
	key := pendingSignupKey(verifyToken)
	raw, err := s.redisClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_VERIFY_TOKEN",
			Message: "Verification link is invalid or has expired",
		}
	}
	if err != nil {
		return nil, err
	}

	var pending pendingSignup
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, err
	}

	// Guard against a race where the same email signed up twice before verifying.
	existing, err := s.repo.FindByEmail(pending.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		// Clean up the stale token and surface a clear error.
		_ = s.redisClient.Del(ctx, key).Err()
		return nil, &middleware.APIError{
			Status:  http.StatusConflict,
			Code:    "USER_EXISTS",
			Message: "An account with this email already exists",
		}
	}

	user, err := s.repo.Create(pending.Email, pending.PasswordHash, pending.Role, pending.SchoolID)
	if err != nil {
		return nil, err
	}

	// Token is consumed — delete it so it cannot be reused.
	_ = s.redisClient.Del(ctx, key).Err()

	return s.checkDeviceAndIssueTokens(ctx, user.ID, user.Email, user.Role, pending.RememberMe, clientInfo)
}

// VerifyDevice looks up the pending device-auth by token, registers the device,
// and returns a full auth response.
func (s *Service) VerifyDevice(ctx context.Context, verifyToken string, clientInfo middleware.ClientInfo) (*AuthResponse, error) {
	key := pendingDeviceAuthKey(verifyToken)
	raw, err := s.redisClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_DEVICE_TOKEN",
			Message: "Device verification link is invalid or has expired",
		}
	}
	if err != nil {
		return nil, err
	}

	var pending pendingDeviceAuth
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, err
	}

	// Token is consumed first to prevent replay.
	_ = s.redisClient.Del(ctx, key).Err()

	if _, err := s.deviceRepo.Create(pending.UserID, pending.Fingerprint, pending.DeviceType, pending.UserAgent); err != nil {
		return nil, err
	}

	ttl := refreshTTLFor(pending.RememberMe)
	pair, err := token.GeneratePair(ctx, pending.UserID, pending.Email, pending.Role, ttl)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   ttl,
		User:         UserData{ID: pending.UserID, Email: pending.Email, Role: pending.Role},
	}, nil
}

func pendingSignupKey(token string) string {
	return "pending_signup:" + token
}

func pendingDeviceAuthKey(token string) string {
	return "pending_device_auth:" + token
}

func (s *Service) SetRole(ctx context.Context, userID uint64, role string) (*AuthResponse, error) {
	if !IsValidSignupRole(role) {
		return nil, &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_ROLE",
			Message: "Role must be one of: student, teacher, principal, teaching_assistant",
		}
	}

	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "USER_NOT_FOUND",
			Message: "User not found",
		}
	}

	if user.Role != RolePending {
		return nil, &middleware.APIError{
			Status:  http.StatusConflict,
			Code:    "ROLE_ALREADY_SET",
			Message: "Role has already been assigned",
		}
	}

	user, err = s.repo.UpdateRole(userID, role)
	if err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role, token.RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   token.RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Refresh validates the given refresh token, rotates it (revoke + issue new),
// and returns a fresh token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	userID, err := token.ValidateRefresh(ctx, refreshToken)
	if err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_REFRESH_TOKEN",
			Message: "Refresh token is invalid or has expired",
		}
	}

	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "USER_NOT_FOUND",
			Message: "User no longer exists",
		}
	}

	if err := token.RevokeRefresh(ctx, refreshToken); err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role, token.RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   token.RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Logout revokes the refresh token, invalidating the session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return token.RevokeRefresh(ctx, refreshToken)
}

func (s *Service) AppleAuthenticate(ctx context.Context, t string, clientInfo middleware.ClientInfo) (*LoginOutcome, error) {
	return nil, nil
}

func (s *Service) MicrosoftAuthenticate(ctx context.Context, idToken string, clientInfo middleware.ClientInfo) (*LoginOutcome, error) {
	claims, err := s.msVerifier.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	addr := claims.Email
	if addr == "" {
		addr = claims.PreferredUsername
	}

	user, err := s.repo.FindOrCreateByMicrosoftID(claims.OID, addr)
	if err != nil {
		return nil, err
	}

	return s.checkDeviceAndIssueTokens(ctx, user.ID, user.Email, user.Role, false, clientInfo)
}

func (s *Service) GoogleAuthenticate(ctx context.Context, idToken string, clientInfo middleware.ClientInfo) (*LoginOutcome, error) {
	claims, err := s.googleVerifier.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindOrCreateByGoogleID(claims.Sub, claims.Email)
	if err != nil {
		return nil, err
	}

	return s.checkDeviceAndIssueTokens(ctx, user.ID, user.Email, user.Role, false, clientInfo)
}

// --- private helpers ---

// checkDeviceAndIssueTokens is the shared post-authentication step. It checks
// whether the requesting device is already known for the user:
//   - Known device: updates LastSeenAt and immediately issues tokens.
//   - Unknown device: stores a short-lived pending record in Redis, sends a
//     verification email, and returns a DeviceChallenge (no tokens yet).
func (s *Service) checkDeviceAndIssueTokens(
	ctx context.Context,
	userID uint64,
	userEmail, role string,
	rememberMe bool,
	clientInfo middleware.ClientInfo,
) (*LoginOutcome, error) {
	fingerprint := device.Fingerprint(clientInfo.UserAgent)

	known, err := s.deviceRepo.FindByUserAndFingerprint(userID, fingerprint)
	if err != nil {
		return nil, err
	}

	if known != nil {
		// Familiar device — update last-seen asynchronously so we don't block
		// the response on a non-critical write.
		go func() {
			_ = s.deviceRepo.UpdateLastSeen(known.ID)
		}()

		ttl := refreshTTLFor(rememberMe)
		pair, err := token.GeneratePair(ctx, userID, userEmail, role, ttl)
		if err != nil {
			return nil, err
		}
		return &LoginOutcome{Auth: &AuthResponse{
			AccessToken:  pair.AccessToken,
			RefreshToken: pair.RefreshToken,
			RefreshTTL:   ttl,
			User:         UserData{ID: userID, Email: userEmail, Role: role},
		}}, nil
	}

	// Unknown device — initiate verification flow.
	verifyToken := uuid.New().String()
	pending := pendingDeviceAuth{
		UserID:      userID,
		Email:       userEmail,
		Role:        role,
		Fingerprint: fingerprint,
		DeviceType:  string(clientInfo.DeviceType),
		UserAgent:   clientInfo.UserAgent,
		RememberMe:  rememberMe,
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return nil, err
	}

	key := pendingDeviceAuthKey(verifyToken)
	if err := s.redisClient.Set(ctx, key, data, pendingDeviceAuthTTL).Err(); err != nil {
		return nil, err
	}

	if s.emailSender != nil {
		verifyURL := fmt.Sprintf("%s/verify-device?token=%s", s.appURL, verifyToken)
		body := fmt.Sprintf(
			"A login was attempted from an unrecognized device.\n\n"+
				"If this was you, verify this device by visiting the link below:\n\n%s\n\n"+
				"This link expires in 15 minutes. If you did not attempt this login, you can ignore this email.",
			verifyURL,
		)
		go email.SendWithRetry(context.Background(), s.emailSender, userEmail, "Verify your device", body)
	}

	return &LoginOutcome{DeviceChallenge: &DeviceChallengeResponse{
		Message: "A verification email has been sent to your address. Please check your inbox to authorize this device.",
	}}, nil
}

// HashPassword generates a bcrypt hash from the given plaintext password.
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword compares a plaintext password against a stored hash.
func (s *Service) ComparePassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

func refreshTTLFor(rememberMe bool) time.Duration {
	if rememberMe {
		return token.RefreshTTLRememberMe
	}
	return token.RefreshTTLDefault
}
