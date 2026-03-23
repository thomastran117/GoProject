package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"backend/internal/app/core/token"
	"backend/internal/config/middleware"

	"golang.org/x/crypto/bcrypt"
)

type AuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserData `json:"user"`
}

type UserData struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Service struct {
	repo           *Repository
	googleClientID string
}

func NewService(repo *Repository, googleClientID string) *Service {
	return &Service{repo: repo, googleClientID: googleClientID}
}

// --- public interface ---

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
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

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

func (s *Service) Signup(ctx context.Context, email, password, role string) (*AuthResponse, error) {
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

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
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

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Logout revokes the refresh token, invalidating the session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return token.RevokeRefresh(ctx, refreshToken)
}

func (s *Service) AppleAuthenticate(ctx context.Context, t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) MicrosoftAuthenticate(ctx context.Context, t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) GoogleAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	claims, err := s.verifyGoogleIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindOrCreateByEmail(claims.Email)
	if err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
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

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"
const googleIssuer1 = "accounts.google.com"
const googleIssuer2 = "https://accounts.google.com"

const googleRetryMax = 3
const googleRetryBase = 100 * time.Millisecond
const googleRetryMaxDelay = 1 * time.Second

type googleTokenInfo struct {
	Aud           string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Iss           string `json:"iss"`
	Exp           string `json:"exp"`
	ErrorDesc     string `json:"error_description"`
}

var errInvalidGoogleToken = &middleware.APIError{
	Status:  http.StatusUnauthorized,
	Code:    "INVALID_GOOGLE_TOKEN",
	Message: "Google token is invalid or expired",
}

// verifyGoogleIDToken calls Google's tokeninfo endpoint to validate the ID token
// and returns the parsed claims on success. Transient errors (network failures,
// 429, 5xx) are retried up to googleRetryMax times with exponential backoff and
// jitter. Permanent errors (4xx) are returned immediately.
func (s *Service) verifyGoogleIDToken(ctx context.Context, idToken string) (*googleTokenInfo, error) {
	var (
		body   googleTokenInfo
		lastErr error
	)

	for attempt := range googleRetryMax {
		if attempt > 0 {
			delay := min(googleRetryBase<<uint(attempt-1), googleRetryMaxDelay)
			// Add up to 50% jitter to avoid thundering herd.
			jitter := time.Duration(rand.Int64N(int64(delay / 2)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleTokenInfoURL+"?id_token="+idToken, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build google tokeninfo request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("google token verification failed: %w", err)
			continue // network error — transient, retry
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			lastErr = fmt.Errorf("google tokeninfo returned %d", resp.StatusCode)
			continue // transient, retry
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, errInvalidGoogleToken // 4xx — permanent, don't retry
		}

		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to parse google token response: %w", err)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("google token verification failed after %d attempts: %w", googleRetryMax, lastErr)
	}

	info := &body

	if info.ErrorDesc != "" {
		return nil, errInvalidGoogleToken
	}

	// Validate issuer explicitly rather than relying solely on Google's check.
	if info.Iss != googleIssuer1 && info.Iss != googleIssuer2 {
		return nil, errInvalidGoogleToken
	}

	// Validate expiry explicitly. Exp is a Unix timestamp string.
	var exp int64
	if _, err := fmt.Sscanf(info.Exp, "%d", &exp); err != nil || exp == 0 {
		return nil, errInvalidGoogleToken
	}
	if time.Now().Unix() >= exp {
		return nil, errInvalidGoogleToken
	}

	if s.googleClientID != "" && info.Aud != s.googleClientID {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_GOOGLE_TOKEN",
			Message: "Google token audience mismatch",
		}
	}

	// Strict boolean check — reject anything that isn't exactly "true".
	if info.EmailVerified != "true" {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "UNVERIFIED_EMAIL",
			Message: "Google account email is not verified",
		}
	}

	return info, nil
}
