package assignment

import (
	"context"
	"math"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// CourseInfo carries the minimal course data needed for ownership checks
// without importing the course package directly.
type CourseInfo struct {
	TeacherID uint64
}

// assignmentRepository is the interface the Service depends on for data
// access, allowing the service to be tested without a real database.
type assignmentRepository interface {
	FindByID(id uint64) (*Assignment, error)
	FindByCourse(courseID uint64, p Page) ([]*Assignment, int64, error)
	Search(f SearchFilter, p Page) ([]*Assignment, int64, error)
	Create(a *Assignment) (*Assignment, error)
	Update(id uint64, fields map[string]any) (*Assignment, error)
	Delete(id uint64) (bool, error)
}

// Service implements the business logic for assignments.
type Service struct {
	repo       assignmentRepository
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error)
}

// NewService creates a Service wired to the given repository and course lookup
// function. findCourse is injected to avoid a circular import with the course
// package; it should return nil, nil when the course does not exist.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
) *Service {
	return &Service{repo: repo, findCourse: findCourse}
}

// --- DTOs ---

// AssignmentResponse is the JSON-serialisable representation of an Assignment
// returned from all service methods.
type AssignmentResponse struct {
	ID          uint64     `json:"id"`
	CourseID    uint64     `json:"course_id"`
	AuthorID    uint64     `json:"author_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	Points      uint       `json:"points"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PaginationMeta holds the pagination metadata included in list responses.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PagedResult wraps a slice of assignment responses together with pagination metadata.
type PagedResult struct {
	Data       []*AssignmentResponse `json:"data"`
	Pagination PaginationMeta        `json:"pagination"`
}

// --- Params ---

// CreateParams carries the validated inputs for creating an assignment.
type CreateParams struct {
	Title       string
	Description string
	DueAt       *time.Time
	Points      uint
	Status      string
}

// UpdateParams carries the validated inputs for updating an assignment.
type UpdateParams struct {
	Title       string
	Description string
	DueAt       *time.Time
	Points      uint
	Status      string
}

// --- Validation ---

var validStatuses = map[string]bool{
	"draft":     true,
	"published": true,
	"closed":    true,
}

func normaliseStatus(s string) (string, error) {
	if s == "" {
		return "draft", nil
	}
	if !validStatuses[s] {
		return "", &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_STATUS",
			Message: "Status must be one of: draft, published, closed",
		}
	}
	return s, nil
}

func clampPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func buildPaginationMeta(page, pageSize int, total int64) PaginationMeta {
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}
	return PaginationMeta{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func toResponse(a *Assignment) *AssignmentResponse {
	return &AssignmentResponse{
		ID:          a.ID,
		CourseID:    a.CourseID,
		AuthorID:    a.AuthorID,
		Title:       a.Title,
		Description: a.Description,
		DueAt:       a.DueAt,
		Points:      a.Points,
		Status:      a.Status,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func toResponses(rows []*Assignment) []*AssignmentResponse {
	out := make([]*AssignmentResponse, len(rows))
	for i, a := range rows {
		out[i] = toResponse(a)
	}
	return out
}

// --- Service methods ---

// Create posts a new assignment to the given course. The caller must be the
// teacher assigned to that course or an admin.
func (s *Service) Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*AssignmentResponse, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "COURSE_NOT_FOUND",
			Message: "Course not found",
		}
	}
	if callerRole != auth.RoleAdmin {
		if callerRole != auth.RoleTeacher || course.TeacherID != callerUserID {
			return nil, &middleware.APIError{
				Status:  http.StatusForbidden,
				Code:    "FORBIDDEN",
				Message: "Only the course teacher or an admin can post assignments",
			}
		}
	}

	status, err := normaliseStatus(p.Status)
	if err != nil {
		return nil, err
	}

	a := &Assignment{
		CourseID:    courseID,
		AuthorID:    callerUserID,
		Title:       p.Title,
		Description: p.Description,
		DueAt:       p.DueAt,
		Points:      p.Points,
		Status:      status,
	}
	created, err := s.repo.Create(a)
	if err != nil {
		return nil, err
	}
	return toResponse(created), nil
}

// GetByCourse returns a paginated list of assignments for the given course.
// Any authenticated user may call this.
func (s *Service) GetByCourse(ctx context.Context, courseID uint64, page, pageSize int) (*PagedResult, error) {
	page, pageSize = clampPage(page, pageSize)
	rows, total, err := s.repo.FindByCourse(courseID, Page{Number: page, Size: pageSize})
	if err != nil {
		return nil, err
	}
	return &PagedResult{
		Data:       toResponses(rows),
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}

// GetByID returns the assignment with the given ID. Any authenticated user may
// call this.
func (s *Service) GetByID(ctx context.Context, id uint64) (*AssignmentResponse, error) {
	a, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ASSIGNMENT_NOT_FOUND",
			Message: "Assignment not found",
		}
	}
	return toResponse(a), nil
}

// Update modifies an existing assignment. The caller must be the original
// author or an admin.
func (s *Service) Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*AssignmentResponse, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ASSIGNMENT_NOT_FOUND",
			Message: "Assignment not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the assignment author or an admin can update this assignment",
		}
	}

	status, err := normaliseStatus(p.Status)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"title":       p.Title,
		"description": p.Description,
		"due_at":      p.DueAt,
		"points":      p.Points,
		"status":      status,
	}
	updated, err := s.repo.Update(id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ASSIGNMENT_NOT_FOUND",
			Message: "Assignment not found",
		}
	}
	return toResponse(updated), nil
}

// Delete removes an assignment. The caller must be the original author or an admin.
func (s *Service) Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ASSIGNMENT_NOT_FOUND",
			Message: "Assignment not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the assignment author or an admin can delete this assignment",
		}
	}
	_, err = s.repo.Delete(id)
	return err
}

// Search returns a paginated, filterable list of all assignments across all
// courses. Restricted to admins.
func (s *Service) Search(ctx context.Context, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only admins can access the global assignments list",
		}
	}
	page, pageSize = clampPage(page, pageSize)
	rows, total, err := s.repo.Search(f, Page{Number: page, Size: pageSize})
	if err != nil {
		return nil, err
	}
	return &PagedResult{
		Data:       toResponses(rows),
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}
