package auth

import (
	"context"
	"net/http"
	"time"

	"backend/internal/config/middleware"

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

type Service struct {
	repo               *Repository
	googleClientID     string
	microsoftClientID  string
	turnstileSecretKey string
	skipTurnstile      bool
	httpClient         *http.Client
	jwksCache          msJWKSCache
}

func NewService(repo *Repository, googleClientID, microsoftClientID, turnstileSecretKey string, skipTurnstile bool) *Service {
	return &Service{
		repo:               repo,
		googleClientID:     googleClientID,
		microsoftClientID:  microsoftClientID,
		turnstileSecretKey: turnstileSecretKey,
		skipTurnstile:      skipTurnstile,
		// Shared across all OAuth requests. The 10-second timeout is a hard
		// cap; per-request context deadlines still take precedence.
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// --- public interface ---

func (s *Service) Login(ctx context.Context, email, password, captcha string, rememberMe bool) (*AuthResponse, error) {
	if !s.skipTurnstile {
		if err := VerifyTurnstile(ctx, s.httpClient, s.turnstileSecretKey, captcha); err != nil {
			return nil, err
		}
	}

	user, err := s.repo.FindByEmail(email)
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

	ttl := refreshTTLFor(rememberMe)
	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, ttl)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   ttl,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

func (s *Service) Signup(ctx context.Context, email, password, captcha, role string, rememberMe bool) (*AuthResponse, error) {
	if !IsValidSignupRole(role) {
		return nil, &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_ROLE",
			Message: "Role must be one of: student, teacher, principal, teaching_assistant",
		}
	}

	if !s.skipTurnstile {
		if err := VerifyTurnstile(ctx, s.httpClient, s.turnstileSecretKey, captcha); err != nil {
			return nil, err
		}
	}

	existing, err := s.repo.FindByEmail(email)
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

	user, err := s.repo.Create(email, string(hash), role)
	if err != nil {
		return nil, err
	}

	ttl := refreshTTLFor(rememberMe)
	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, ttl)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   ttl,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
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

	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Refresh validates the given refresh token, rotates it (revoke + issue new),
// and returns a fresh token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	userID, err := ValidateRefresh(ctx, refreshToken)
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

	if err := RevokeRefresh(ctx, refreshToken); err != nil {
		return nil, err
	}

	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Logout revokes the refresh token, invalidating the session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return RevokeRefresh(ctx, refreshToken)
}

func (s *Service) AppleAuthenticate(ctx context.Context, t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) MicrosoftAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	claims, err := s.verifyMicrosoftIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	user, err := s.repo.FindOrCreateByMicrosoftID(claims.OID, email)
	if err != nil {
		return nil, err
	}

	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

func (s *Service) GoogleAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	claims, err := s.verifyGoogleIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindOrCreateByGoogleID(claims.Sub, claims.Email)
	if err != nil {
		return nil, err
	}

	pair, err := GeneratePair(ctx, user.ID, user.Email, user.Role, RefreshTTLDefault)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		RefreshTTL:   RefreshTTLDefault,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// --- private helpers ---

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
		return RefreshTTLRememberMe
	}
	return RefreshTTLDefault
}
