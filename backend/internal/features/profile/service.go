package profile

import (
	"context"
	"net/http"
	"time"

	"backend/internal/config/middleware"
)

// ProfileResponse is the public-facing DTO returned by every service method.
// It omits internal fields like PasswordHash and exposes only what the API
// consumer needs.
type ProfileResponse struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service holds the business logic for the profile feature and depends on a
// Repository for all persistence operations.
type Service struct {
	repo *Repository
}

// NewService creates a Service using the provided Repository.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetByID returns the profile matching the given id. Returns a 404 APIError
// when no profile exists with that id.
func (s *Service) GetByID(ctx context.Context, id uint64) (*ProfileResponse, error) {
	p, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "PROFILE_NOT_FOUND", Message: "Profile not found"}
	}
	return toResponse(p), nil
}

// GetAll returns every profile in the system. Returns an empty slice when none
// exist.
func (s *Service) GetAll(ctx context.Context) ([]*ProfileResponse, error) {
	profiles, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return toResponses(profiles), nil
}

// GetByIDs returns all profiles whose id is in the provided slice. Profiles
// that do not exist are silently omitted; the caller should compare lengths if
// exact presence is required.
func (s *Service) GetByIDs(ctx context.Context, ids []uint64) ([]*ProfileResponse, error) {
	profiles, err := s.repo.FindByIDs(ids)
	if err != nil {
		return nil, err
	}
	return toResponses(profiles), nil
}

// Create persists a new profile associated with the given userID and returns
// the created record.
func (s *Service) Create(ctx context.Context, userID uint64, username, avatarURL string) (*ProfileResponse, error) {
	p, err := s.repo.Create(userID, username, avatarURL)
	if err != nil {
		return nil, err
	}
	return toResponse(p), nil
}

// Update replaces the username and avatar URL of the profile identified by id.
// Returns a 404 APIError when no profile with that id exists.
func (s *Service) Update(ctx context.Context, id uint64, username, avatarURL string) (*ProfileResponse, error) {
	p, err := s.repo.Update(id, username, avatarURL)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "PROFILE_NOT_FOUND", Message: "Profile not found"}
	}
	return toResponse(p), nil
}

// Delete removes the profile with the given id. Returns a 404 APIError when no
// profile with that id exists.
func (s *Service) Delete(ctx context.Context, id uint64) error {
	found, err := s.repo.Delete(id)
	if err != nil {
		return err
	}
	if !found {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "PROFILE_NOT_FOUND", Message: "Profile not found"}
	}
	return nil
}

// toResponse maps a Profile model to its public DTO.
func toResponse(p *Profile) *ProfileResponse {
	return &ProfileResponse{
		ID:        p.ID,
		UserID:    p.UserID,
		Username:  p.Username,
		AvatarURL: p.AvatarURL,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// toResponses maps a slice of Profile models to their public DTOs.
func toResponses(profiles []*Profile) []*ProfileResponse {
	result := make([]*ProfileResponse, len(profiles))
	for i, p := range profiles {
		result[i] = toResponse(p)
	}
	return result
}
