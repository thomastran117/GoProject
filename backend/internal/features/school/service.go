package school

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// SchoolResponse is the public-facing DTO returned by every service method.
type SchoolResponse struct {
	ID          uint64    `json:"id"`
	PrincipalID uint64    `json:"principal_id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	City        string    `json:"city"`
	Country     string    `json:"country"`
	Phone       string    `json:"phone"`
	Email       string    `json:"email"`
	Website     string    `json:"website"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type schoolRepository interface {
	FindByID(id uint64) (*School, error)
	FindAll() ([]*School, error)
	FindByIDs(ids []uint64) ([]*School, error)
	Search(f SearchFilter) ([]*School, error)
	Create(principalID uint64, name, address, city, country, phone, email, website string) (*School, error)
	Update(id uint64, name, address, city, country, phone, email, website string) (*School, error)
	Delete(id uint64) (bool, error)
}

// Service holds the business logic for the school feature.
type Service struct {
	repo schoolRepository
}

// NewService creates a Service using the provided Repository.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetByID returns the school matching the given id. Returns a 404 APIError
// when no school exists with that id.
func (s *Service) GetByID(ctx context.Context, id uint64) (*SchoolResponse, error) {
	school, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if school == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	return toResponse(school), nil
}

// GetAll returns every school in the system. Returns an empty slice when none
// exist.
func (s *Service) GetAll(ctx context.Context) ([]*SchoolResponse, error) {
	schools, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return toResponses(schools), nil
}

// GetByIDs returns all schools whose id is in the provided slice. Schools that
// do not exist are silently omitted.
func (s *Service) GetByIDs(ctx context.Context, ids []uint64) ([]*SchoolResponse, error) {
	schools, err := s.repo.FindByIDs(ids)
	if err != nil {
		return nil, err
	}
	return toResponses(schools), nil
}

// Search returns schools matching the provided filter. Any zero-value field
// in the filter is ignored (not applied as a predicate).
func (s *Service) Search(ctx context.Context, f SearchFilter) ([]*SchoolResponse, error) {
	schools, err := s.repo.Search(f)
	if err != nil {
		return nil, err
	}
	return toResponses(schools), nil
}

// Create persists a new school owned by callerUserID. Only principals may
// create schools; any other role receives a 403 APIError.
func (s *Service) Create(ctx context.Context, callerUserID uint64, callerRole, name, address, city, country, phone, email, website string) (*SchoolResponse, error) {
	if callerRole != auth.RolePrincipal {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only principals can create schools"}
	}
	school, err := s.repo.Create(callerUserID, name, address, city, country, phone, email, website)
	if err != nil {
		return nil, err
	}
	return toResponse(school), nil
}

// Update replaces the mutable fields of the school identified by id. The caller
// must be the principal who owns the school; otherwise a 403 APIError is
// returned. Returns a 404 APIError when no school with that id exists.
func (s *Service) Update(ctx context.Context, id, callerUserID uint64, callerRole, name, address, city, country, phone, email, website string) (*SchoolResponse, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	if callerRole != auth.RolePrincipal || existing.PrincipalID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the owning principal can update this school"}
	}
	school, err := s.repo.Update(id, name, address, city, country, phone, email, website)
	if err != nil {
		return nil, err
	}
	if school == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	return toResponse(school), nil
}

// Delete removes the school with the given id. The caller must be the principal
// who owns the school; otherwise a 403 APIError is returned. Returns a 404
// APIError when no school with that id exists.
func (s *Service) Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	if callerRole != auth.RolePrincipal || existing.PrincipalID != callerUserID {
		return &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the owning principal can delete this school"}
	}
	found, err := s.repo.Delete(id)
	if err != nil {
		return err
	}
	if !found {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	return nil
}

func toResponse(s *School) *SchoolResponse {
	return &SchoolResponse{
		ID:          s.ID,
		PrincipalID: s.PrincipalID,
		Name:        s.Name,
		Address:     s.Address,
		City:        s.City,
		Country:     s.Country,
		Phone:       s.Phone,
		Email:       s.Email,
		Website:     s.Website,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

func toResponses(schools []*School) []*SchoolResponse {
	result := make([]*SchoolResponse, len(schools))
	for i, s := range schools {
		result[i] = toResponse(s)
	}
	return result
}
