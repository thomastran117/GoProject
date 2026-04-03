package lecture

import (
	"context"
	"math"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
	"backend/internal/utilities/logger"
)

// CourseInfo carries the minimal course data needed for ownership checks
// without importing the course package directly.
type CourseInfo struct {
	TeacherID uint64
}

// lectureRepository is the interface the Service depends on for data access.
type lectureRepository interface {
	FindByID(id uint64) (*Lecture, error)
	FindByCourse(courseID uint64, p Page) ([]*Lecture, int64, error)
	Search(f SearchFilter, p Page) ([]*Lecture, int64, error)
	Create(l *Lecture) (*Lecture, error)
	Update(id uint64, fields map[string]any) (*Lecture, error)
	Delete(id uint64) (bool, error)
	MarkViewed(userID, lectureID uint64) error
	FindViewedIDs(userID uint64, ids []uint64) (map[uint64]bool, error)
}

// Service implements the business logic for lectures.
type Service struct {
	repo       lectureRepository
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error)
}

// NewService creates a Service wired to the given repository, course lookup,
// and enrollment check functions. Both are injected to avoid circular imports.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
) *Service {
	return &Service{repo: repo, findCourse: findCourse, isEnrolled: isEnrolled}
}

// --- DTOs ---

// LectureResponse is the JSON-serialisable representation of a Lecture.
type LectureResponse struct {
	ID        uint64    `json:"id"`
	CourseID  uint64    `json:"course_id"`
	AuthorID  uint64    `json:"author_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsViewed  bool      `json:"is_viewed"`
}

// PaginationMeta holds the pagination metadata included in list responses.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PagedResult wraps a slice of lecture responses together with pagination metadata.
type PagedResult struct {
	Data       []*LectureResponse `json:"data"`
	Pagination PaginationMeta     `json:"pagination"`
}

// --- Params ---

// CreateParams carries the validated inputs for creating a lecture.
type CreateParams struct {
	Title   string
	Content string
}

// UpdateParams carries the validated inputs for updating a lecture.
type UpdateParams struct {
	Title   string
	Content string
}

// --- Helpers ---

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

func toResponse(l *Lecture, isViewed bool) *LectureResponse {
	return &LectureResponse{
		ID:        l.ID,
		CourseID:  l.CourseID,
		AuthorID:  l.AuthorID,
		Title:     l.Title,
		Content:   l.Content,
		CreatedAt: l.CreatedAt,
		UpdatedAt: l.UpdatedAt,
		IsViewed:  isViewed,
	}
}


// checkReadAccess returns a FORBIDDEN error if the caller is not the course
// teacher, an admin, or actively enrolled in the course.
// course may be nil (e.g. data inconsistency); a nil course is treated as
// "caller is not the teacher" and falls through to the enrollment check.
func (s *Service) checkReadAccess(ctx context.Context, courseID, callerUserID uint64, callerRole string, course *CourseInfo) error {
	if callerRole == auth.RoleAdmin {
		return nil
	}
	if course != nil && course.TeacherID == callerUserID {
		return nil
	}
	enrolled, err := s.isEnrolled(ctx, courseID, callerUserID)
	if err != nil {
		return err
	}
	if !enrolled {
		return &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "You must be enrolled in this course to view its lectures",
		}
	}
	return nil
}

// --- Service methods ---

// Create posts a new lecture to the given course. The caller must be the
// teacher assigned to that course or an admin.
func (s *Service) Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*LectureResponse, error) {
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
				Message: "Only the course teacher or an admin can post lectures",
			}
		}
	}

	l := &Lecture{
		CourseID: courseID,
		AuthorID: callerUserID,
		Title:    p.Title,
		Content:  p.Content,
	}
	created, err := s.repo.Create(l)
	if err != nil {
		return nil, err
	}
	return toResponse(created, false), nil
}

// GetByCourse returns a paginated list of lectures for the given course.
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
	if err := s.checkReadAccess(ctx, courseID, callerUserID, callerRole, course); err != nil {
		return nil, err
	}
	page, pageSize = clampPage(page, pageSize)
	rows, total, err := s.repo.FindByCourse(courseID, Page{Number: page, Size: pageSize})
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, len(rows))
	for i, l := range rows {
		ids[i] = l.ID
	}
	viewedSet, err := s.repo.FindViewedIDs(callerUserID, ids)
	if err != nil {
		return nil, err
	}
	data := make([]*LectureResponse, len(rows))
	for i, l := range rows {
		data[i] = toResponse(l, viewedSet[l.ID])
	}
	return &PagedResult{
		Data:       data,
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}

// GetByID returns the lecture with the given ID.
// The caller must be enrolled in the lecture's course, the course teacher, or an admin.
func (s *Service) GetByID(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*LectureResponse, error) {
	l, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "LECTURE_NOT_FOUND",
			Message: "Lecture not found",
		}
	}
	if callerRole != auth.RoleAdmin {
		course, err := s.findCourse(ctx, l.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil {
			return nil, &middleware.APIError{
				Status:  http.StatusForbidden,
				Code:    "FORBIDDEN",
				Message: "You must be enrolled in this course to view its lectures",
			}
		}
		if err := s.checkReadAccess(ctx, l.CourseID, callerUserID, callerRole, course); err != nil {
			return nil, err
		}
	}
	if err := s.repo.MarkViewed(callerUserID, l.ID); err != nil {
		logger.Warn("lecture: failed to record view for user %d lecture %d: %v", callerUserID, l.ID, err)
	}
	return toResponse(l, true), nil
}

// Update modifies an existing lecture. The caller must be the original author or an admin.
func (s *Service) Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*LectureResponse, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "LECTURE_NOT_FOUND",
			Message: "Lecture not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the lecture author or an admin can update this lecture",
		}
	}

	fields := map[string]any{
		"title":   p.Title,
		"content": p.Content,
	}
	updated, err := s.repo.Update(id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "LECTURE_NOT_FOUND",
			Message: "Lecture not found",
		}
	}
	return toResponse(updated, false), nil
}

// Delete removes a lecture. The caller must be the original author or an admin.
func (s *Service) Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "LECTURE_NOT_FOUND",
			Message: "Lecture not found",
		}
	}
	if callerRole != auth.RoleAdmin && existing.AuthorID != callerUserID {
		return &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the lecture author or an admin can delete this lecture",
		}
	}
	_, err = s.repo.Delete(id)
	return err
}

// Search returns a paginated, filterable list of all lectures across all courses.
// Restricted to admins.
func (s *Service) Search(ctx context.Context, callerUserID uint64, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only admins can access the global lectures list",
		}
	}
	page, pageSize = clampPage(page, pageSize)
	rows, total, err := s.repo.Search(f, Page{Number: page, Size: pageSize})
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, len(rows))
	for i, l := range rows {
		ids[i] = l.ID
	}
	viewedSet, err := s.repo.FindViewedIDs(callerUserID, ids)
	if err != nil {
		return nil, err
	}
	data := make([]*LectureResponse, len(rows))
	for i, l := range rows {
		data[i] = toResponse(l, viewedSet[l.ID])
	}
	return &PagedResult{
		Data:       data,
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}
