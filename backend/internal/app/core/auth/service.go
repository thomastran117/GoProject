package auth

import (
	"context"
	"net/http"

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
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
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

func (s *Service) AppleAuthenticate(t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) MicrosoftAuthenticate(t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) GoogleAuthenticate(t string) (*AuthResponse, error) {
	return nil, nil
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

func (s *Service) VerifyGoogleToken(t string) (string, error) {
	return "yes", nil
}

func (s *Service) VerifyMicrosoftToken(t string) (string, error) {
	return "yes", nil
}

func (s *Service) VerifyAppleToken(t string) (string, error) {
	return "yes", nil
}
