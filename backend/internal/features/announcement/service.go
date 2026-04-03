package announcement

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

// announcementRepository is the interface the Service depends on for data
// access, allowing the service to be tested without a real database.
type announcementRepository interface {
	FindByID(id uint64) (*Announcement, error)
	FindByCourse(courseID uint64, p Page) ([]*Announcement, int64, error)
	Search(f SearchFilter, p Page) ([]*Announcement, int64, error)
	Create(a *Announcement) (*Announcement, error)
	Update(id uint64, fields map[string]any) (*Announcement, error)
	Delete(id uint64) (bool, error)
}

// Service implements the business logic for announcements.
type Service struct {
	repo       announcementRepository
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error)
}

// NewService creates a Service wired to the given repository and course lookup
// function. findCourse is injected to avoid a circular import with the course
// package; it should return nil, nil when the course does not exist.
// isEnrolled is injected to avoid a circular import with the enrollment package.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
) *Service {
	return &Service{repo: repo, findCourse: findCourse, isEnrolled: isEnrolled}
}

// --- DTOs ---

// AnnouncementResponse is the JSON-serialisable representation of an
// Announcement that is returned from all service methods.
type AnnouncementResponse struct {
	ID          uint64     `json:"id"`
	CourseID    uint64     `json:"course_id"`
	AuthorID    uint64     `json:"author_id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Priority    string     `json:"priority"`
	IsPinned    bool       `json:"is_pinned"`
	PublishedAt *time.Time `json:"published_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
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

// PagedResult wraps a slice of announcement responses together with pagination
// metadata.
type PagedResult struct {
	Data       []*AnnouncementResponse `json:"data"`
	Pagination PaginationMeta          `json:"pagination"`
}

// --- Params ---

// CreateParams carries the validated inputs for creating an announcement.
type CreateParams struct {
	Title       string
	Body        string
	Priority    string
	IsPinned    bool
	PublishedAt *time.Time
	ExpiresAt   *time.Time
}

// UpdateParams carries the validated inputs for updating an announcement.
type UpdateParams struct {
	Title       string
	Body        string
	Priority    string
	IsPinned    bool
	PublishedAt *time.Time
	ExpiresAt   *time.Time
}

// --- Validation ---

var validPriorities = map[string]bool{
	"normal": true,
	"high":   true,
	"urgent": true,
}

func normalisePriority(p string) (string, error) {
	if p == "" {
		return "normal", nil
	}
	if !validPriorities[p] {
		return "", &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_PRIORITY",
			Message: "Priority must be one of: normal, high, urgent",
		}
	}
	return p, nil
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

func toResponse(a *Announcement) *AnnouncementResponse {
	return &AnnouncementResponse{
		ID:          a.ID,
		CourseID:    a.CourseID,
		AuthorID:    a.AuthorID,
		Title:       a.Title,
		Body:        a.Body,
		Priority:    a.Priority,
		IsPinned:    a.IsPinned,
		PublishedAt: a.PublishedAt,
		ExpiresAt:   a.ExpiresAt,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func toResponses(rows []*Announcement) []*AnnouncementResponse {
	out := make([]*AnnouncementResponse, len(rows))
	for i, a := range rows {
		out[i] = toResponse(a)
	}
	return out
}

// --- Service methods ---

// Create posts a new announcement to the given course. The caller must be the
// teacher assigned to that course or an admin.
func (s *Service) Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*AnnouncementResponse, error) {
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
				Message: "Only the course teacher or an admin can post announcements",
			}
		}
	}

	priority, err := normalisePriority(p.Priority)
	if err != nil {
		return nil, err
	}

	a := &Announcement{
		CourseID:    courseID,
		AuthorID:    callerUserID,
		Title:       p.Title,
		Body:        p.Body,
		Priority:    priority,
		IsPinned:    p.IsPinned,
		PublishedAt: p.PublishedAt,
		ExpiresAt:   p.ExpiresAt,
	}
	created, err := s.repo.Create(a)
	if err != nil {
		return nil, err
	}
	return toResponse(created), nil
}

// GetByCourse returns a paginated list of announcements for the given course.
// The caller must be enrolled in the course, the course teacher, or an admin.
func (s *Service) GetByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, page, pageSize int) (*PagedResult, error) {
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
	if callerRole != auth.RoleAdmin && course.TeacherID != callerUserID {
		enrolled, err := s.isEnrolled(ctx, courseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{
				Status:  http.StatusForbidden,
				Code:    "FORBIDDEN",
				Message: "You must be enrolled in this course to view its announcements",
			}
		}
	}
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

// GetByID returns the announcement with the given ID.
// The caller must be enrolled in the announcement's course, the course teacher, or an admin.
func (s *Service) GetByID(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*AnnouncementResponse, error) {
	a, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ANNOUNCEMENT_NOT_FOUND",
			Message: "Announcement not found",
		}
	}
	if callerRole != auth.RoleAdmin {
		course, err := s.findCourse(ctx, a.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil || course.TeacherID != callerUserID {
			enrolled, err := s.isEnrolled(ctx, a.CourseID, callerUserID)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				return nil, &middleware.APIError{
					Status:  http.StatusForbidden,
					Code:    "FORBIDDEN",
					Message: "You must be enrolled in this course to view its announcements",
				}
			}
		}
	}
	return toResponse(a), nil
}

// Update modifies an existing announcement. The caller must be the original
// author or an admin.
func (s *Service) Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*AnnouncementResponse, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ANNOUNCEMENT_NOT_FOUND",
			Message: "Announcement not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the announcement author or an admin can update this announcement",
		}
	}

	priority, err := normalisePriority(p.Priority)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"title":        p.Title,
		"body":         p.Body,
		"priority":     priority,
		"is_pinned":    p.IsPinned,
		"published_at": p.PublishedAt,
		"expires_at":   p.ExpiresAt,
	}
	updated, err := s.repo.Update(id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ANNOUNCEMENT_NOT_FOUND",
			Message: "Announcement not found",
		}
	}
	return toResponse(updated), nil
}

// Delete removes an announcement. The caller must be the original author or an
// admin.
func (s *Service) Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "ANNOUNCEMENT_NOT_FOUND",
			Message: "Announcement not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the announcement author or an admin can delete this announcement",
		}
	}
	_, err = s.repo.Delete(id)
	return err
}

// Search returns a paginated, filterable list of all announcements across all
// courses. Restricted to admins.
func (s *Service) Search(ctx context.Context, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only admins can access the global announcements list",
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
