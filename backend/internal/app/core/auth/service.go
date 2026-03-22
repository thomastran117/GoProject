package auth

import (
	"net/http"

	"backend/internal/config/middleware"
	"backend/internal/app/core/token"

	"golang.org/x/crypto/bcrypt"
)

type AuthResponse struct {
	Token string   `json:"token"`
	User  UserData `json:"user"`
}

type UserData struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Login(email, password string) (*AuthResponse, error) {
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

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid email or password",
		}
	}

	t, err := token.Generate(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token: t,
		User:  UserData{ID: user.ID, Email: user.Email},
	}, nil
}

func (s *Service) Signup(email, password string) (*AuthResponse, error) {
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

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.Create(email, string(hash))
	if err != nil {
		return nil, err
	}

	t, err := token.Generate(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token: t,
		User:  UserData{ID: user.ID, Email: user.Email},
	}, nil
}
