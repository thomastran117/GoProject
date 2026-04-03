package course

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// SchoolInfo carries the minimal school data needed by the course service to
// perform principal-ownership checks without importing the school package.
type SchoolInfo struct {
	PrincipalID uint64
}

// CourseResponse is the public-facing DTO returned by every service method.
type CourseResponse struct {
	ID            uint64     `json:"id"`
	SchoolID      uint64     `json:"school_id"`
	TeacherID     uint64     `json:"teacher_id"`
	Name          string     `json:"name"`
	Code          string     `json:"code"`
	Description   string     `json:"description"`
	Subject       string     `json:"subject"`
	GradeLevel    string     `json:"grade_level"`
	Language      string     `json:"language"`
	Room          string     `json:"room"`
	Schedule      string     `json:"schedule"`
	MaxEnrollment uint       `json:"max_enrollment"`
	Credits       uint       `json:"credits"`
	Status        string     `json:"status"`
	Visibility    string     `json:"visibility"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreateParams holds all fields required to create a new course.
type CreateParams struct {
	SchoolID      uint64
	TeacherID     uint64
	Name          string
	Code          string
	Description   string
	Subject       string
	GradeLevel    string
	Language      string
	Room          string
	Schedule      string
	MaxEnrollment uint
	Credits       uint
	Status        string
	Visibility    string
	StartDate     *time.Time
	EndDate       *time.Time
}

// UpdateParams holds all mutable course fields. SchoolID is intentionally
// excluded — courses cannot be reassigned to a different school.
type UpdateParams struct {
	TeacherID     uint64
	Name          string
	Code          string
	Description   string
	Subject       string
	GradeLevel    string
	Language      string
	Room          string
	Schedule      string
	MaxEnrollment uint
	Credits       uint
	Status        string
	Visibility    string
	StartDate     *time.Time
	EndDate       *time.Time
}

var validStatuses = map[string]bool{
	"active":   true,
	"inactive": true,
	"archived": true,
}

var validVisibilities = map[string]bool{
	"public":  true,
	"private": true,
}

type courseRepository interface {
	FindByID(id uint64) (*Course, error)
	FindByIDs(ids []uint64) ([]*Course, error)
	FindBySchoolAndCode(schoolID uint64, code string) (*Course, error)
	Search(f SearchFilter) ([]*Course, error)
	Create(c *Course) (*Course, error)
	Update(id uint64, fields map[string]any) (*Course, error)
	Delete(id uint64) (bool, error)
}

// Service holds the business logic for the course feature.
type Service struct {
	repo          courseRepository
	schoolExists  func(ctx context.Context, id uint64) (bool, error)
	teacherExists func(ctx context.Context, id uint64) (bool, error)
	findSchool    func(ctx context.Context, id uint64) (*SchoolInfo, error)
}

// NewService creates a Service wired to the given repository and dependency
// closures. schoolExists and findSchool are used for school validation and
// ownership checks; teacherExists validates that a user exists with the
// teacher role.
func NewService(
	repo *Repository,
	schoolExists func(ctx context.Context, id uint64) (bool, error),
	teacherExists func(ctx context.Context, id uint64) (bool, error),
	findSchool func(ctx context.Context, id uint64) (*SchoolInfo, error),
) *Service {
	return &Service{
		repo:          repo,
		schoolExists:  schoolExists,
		teacherExists: teacherExists,
		findSchool:    findSchool,
	}
}

// GetByID returns the course matching the given id. Returns a 404 APIError
// when no course exists with that id.
func (s *Service) GetByID(ctx context.Context, id uint64) (*CourseResponse, error) {
	course, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}
	return toResponse(course), nil
}

// GetByIDs returns all courses whose id is in the provided slice. Courses
// that do not exist are silently omitted.
func (s *Service) GetByIDs(ctx context.Context, ids []uint64) ([]*CourseResponse, error) {
	courses, err := s.repo.FindByIDs(ids)
	if err != nil {
		return nil, err
	}
	return toResponses(courses), nil
}

// Search returns courses matching the provided filter. Any zero-value field
// in the filter is ignored.
func (s *Service) Search(ctx context.Context, f SearchFilter) ([]*CourseResponse, error) {
	courses, err := s.repo.Search(f)
	if err != nil {
		return nil, err
	}
	return toResponses(courses), nil
}

// Create persists a new course. Only principals (who own the target school)
// and admins may create courses. The teacher must exist and carry the teacher
// role.
func (s *Service) Create(ctx context.Context, callerUserID uint64, callerRole string, p CreateParams) (*CourseResponse, error) {
	if callerRole != auth.RolePrincipal && callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only principals or admins can create courses"}
	}

	if !validStatuses[p.Status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "Status must be one of: active, inactive, archived"}
	}

	if p.Visibility == "" {
		p.Visibility = "public"
	}
	if !validVisibilities[p.Visibility] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_VISIBILITY", Message: "Visibility must be one of: public, private"}
	}

	school, err := s.findSchool(ctx, p.SchoolID)
	if err != nil {
		return nil, err
	}
	if school == nil {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "SCHOOL_NOT_FOUND", Message: "School not found"}
	}
	if callerRole == auth.RolePrincipal && school.PrincipalID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You do not own this school"}
	}

	ok, err := s.teacherExists(ctx, p.TeacherID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "TEACHER_NOT_FOUND", Message: "Teacher not found or user is not a teacher"}
	}

	c := &Course{
		SchoolID:      p.SchoolID,
		TeacherID:     p.TeacherID,
		Name:          p.Name,
		Code:          p.Code,
		Description:   p.Description,
		Subject:       p.Subject,
		GradeLevel:    p.GradeLevel,
		Language:      p.Language,
		Room:          p.Room,
		Schedule:      p.Schedule,
		MaxEnrollment: p.MaxEnrollment,
		Credits:       p.Credits,
		Status:        p.Status,
		Visibility:    p.Visibility,
		StartDate:     p.StartDate,
		EndDate:       p.EndDate,
	}
	created, err := s.repo.Create(c)
	if err != nil {
		return nil, err
	}
	return toResponse(created), nil
}

// Update replaces the mutable fields of the course identified by id.
// Authorized callers: admin, the principal who owns the course's school, or
// the teacher currently assigned to the course.
func (s *Service) Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*CourseResponse, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if callerRole != auth.RoleAdmin {
		if callerRole == auth.RoleTeacher && existing.TeacherID == callerUserID {
			// assigned teacher — allowed
		} else if callerRole == auth.RolePrincipal {
			school, err := s.findSchool(ctx, existing.SchoolID)
			if err != nil {
				return nil, err
			}
			if school == nil || school.PrincipalID != callerUserID {
				return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You do not own the school this course belongs to"}
			}
		} else {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You are not authorized to update this course"}
		}
	}

	if !validStatuses[p.Status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "Status must be one of: active, inactive, archived"}
	}

	if p.Visibility == "" {
		p.Visibility = "public"
	}
	if !validVisibilities[p.Visibility] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_VISIBILITY", Message: "Visibility must be one of: public, private"}
	}

	if p.TeacherID != existing.TeacherID {
		ok, err := s.teacherExists(ctx, p.TeacherID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "TEACHER_NOT_FOUND", Message: "Teacher not found or user is not a teacher"}
		}
	}

	if p.Code != existing.Code {
		conflict, err := s.repo.FindBySchoolAndCode(existing.SchoolID, p.Code)
		if err != nil {
			return nil, err
		}
		if conflict != nil {
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "COURSE_CODE_CONFLICT", Message: "A course with that code already exists in this school"}
		}
	}

	fields := map[string]any{
		"teacher_id":     p.TeacherID,
		"name":           p.Name,
		"code":           p.Code,
		"description":    p.Description,
		"subject":        p.Subject,
		"grade_level":    p.GradeLevel,
		"language":       p.Language,
		"room":           p.Room,
		"schedule":       p.Schedule,
		"max_enrollment": p.MaxEnrollment,
		"credits":        p.Credits,
		"status":         p.Status,
		"visibility":     p.Visibility,
		"start_date":     p.StartDate,
		"end_date":       p.EndDate,
	}
	updated, err := s.repo.Update(id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}
	return toResponse(updated), nil
}

// Delete removes the course with the given id. Only the owning principal or
// an admin may delete a course.
func (s *Service) Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if callerRole != auth.RoleAdmin {
		if callerRole != auth.RolePrincipal {
			return &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only principals or admins can delete courses"}
		}
		school, err := s.findSchool(ctx, existing.SchoolID)
		if err != nil {
			return err
		}
		if school == nil || school.PrincipalID != callerUserID {
			return &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You do not own the school this course belongs to"}
		}
	}

	found, err := s.repo.Delete(id)
	if err != nil {
		return err
	}
	if !found {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}
	return nil
}

func toResponse(c *Course) *CourseResponse {
	return &CourseResponse{
		ID:            c.ID,
		SchoolID:      c.SchoolID,
		TeacherID:     c.TeacherID,
		Name:          c.Name,
		Code:          c.Code,
		Description:   c.Description,
		Subject:       c.Subject,
		GradeLevel:    c.GradeLevel,
		Language:      c.Language,
		Room:          c.Room,
		Schedule:      c.Schedule,
		MaxEnrollment: c.MaxEnrollment,
		Credits:       c.Credits,
		Status:        c.Status,
		Visibility:    c.Visibility,
		StartDate:     c.StartDate,
		EndDate:       c.EndDate,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

func toResponses(courses []*Course) []*CourseResponse {
	result := make([]*CourseResponse, len(courses))
	for i, c := range courses {
		result[i] = toResponse(c)
	}
	return result
}
