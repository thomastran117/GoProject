package enrollment

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// CourseInfo carries the minimal course data needed by the enrollment service
// without importing the course package (avoids circular imports).
type CourseInfo struct {
	TeacherID     uint64
	Visibility    string // "public" or "private"
	MaxEnrollment uint
}

// EnrollmentResponse is the public-facing DTO for an enrollment record.
type EnrollmentResponse struct {
	ID         uint64    `json:"id"`
	CourseID   uint64    `json:"course_id"`
	UserID     uint64    `json:"user_id"`
	Status     string    `json:"status"`
	EnrolledAt time.Time `json:"enrolled_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CourseInviteResponse is the public-facing DTO for a Redis-stored invite.
type CourseInviteResponse struct {
	Code      string    `json:"code"`
	CourseID  uint64    `json:"course_id"`
	InviteeID uint64    `json:"invitee_id"`
	InviterID uint64    `json:"inviter_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// PaginationMeta holds pagination metadata returned alongside list responses.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PagedEnrollments wraps a slice of EnrollmentResponse with pagination info.
type PagedEnrollments struct {
	Data       []*EnrollmentResponse `json:"data"`
	Pagination PaginationMeta        `json:"pagination"`
}

type enrollmentRepository interface {
	FindEnrollment(courseID, userID uint64) (*Enrollment, error)
	FindEnrollmentsByCourse(courseID uint64, page Page) ([]*Enrollment, int64, error)
	FindEnrollmentsByUser(userID uint64, page Page) ([]*Enrollment, int64, error)
	CountEnrollmentsByCourse(courseID uint64) (int64, error)
	CreateEnrollment(e *Enrollment) (*Enrollment, error)
	UpdateEnrollmentStatus(courseID, userID uint64, status string) (bool, error)
	FindInviteByCode(ctx context.Context, code string) (*CourseInvite, error)
	FindInviteByCourseAndInvitee(ctx context.Context, courseID, inviteeID uint64) (*CourseInvite, error)
	FindInvitesByCourse(ctx context.Context, courseID uint64) ([]*CourseInvite, error)
	CreateInvite(ctx context.Context, inv *CourseInvite) error
	UpdateInviteStatus(ctx context.Context, code, status string) (*CourseInvite, error)
	AcceptInviteAndEnroll(ctx context.Context, code string, e *Enrollment) (*CourseInvite, *Enrollment, error)
}

// Service holds the business logic for enrollment and invite operations.
type Service struct {
	repo       enrollmentRepository
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error)
	userExists func(ctx context.Context, id uint64) (bool, error)
}

// NewService creates a Service wired to the given repository and dependency
// closures. findCourse and userExists avoid circular package imports.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	userExists func(ctx context.Context, id uint64) (bool, error),
) *Service {
	return &Service{repo: repo, findCourse: findCourse, userExists: userExists}
}

// Enroll enrolls the calling user in the given course. Public courses allow
// direct enrollment; private courses require a pre-accepted invite.
func (s *Service) Enroll(ctx context.Context, callerUserID uint64, courseID uint64) (*EnrollmentResponse, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if course.MaxEnrollment > 0 {
		count, err := s.repo.CountEnrollmentsByCourse(courseID)
		if err != nil {
			return nil, err
		}
		if count >= int64(course.MaxEnrollment) {
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ENROLLMENT_FULL", Message: "This course has reached its maximum enrollment"}
		}
	}

	existing, err := s.repo.FindEnrollment(courseID, callerUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Status == "active" {
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ALREADY_ENROLLED", Message: "You are already enrolled in this course"}
		}
		// Reactivate a dropped enrollment.
		if _, err := s.repo.UpdateEnrollmentStatus(courseID, callerUserID, "active"); err != nil {
			return nil, err
		}
		existing.Status = "active"
		return toEnrollmentResponse(existing), nil
	}

	if course.Visibility == "private" {
		invite, err := s.repo.FindInviteByCourseAndInvitee(ctx, courseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if invite == nil || invite.Status != "accepted" {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "INVITE_REQUIRED", Message: "This is a private course. You must accept an invite to enroll"}
		}
	}

	e := &Enrollment{
		CourseID:   courseID,
		UserID:     callerUserID,
		Status:     "active",
		EnrolledAt: time.Now(),
	}
	created, err := s.repo.CreateEnrollment(e)
	if err != nil {
		return nil, err
	}
	return toEnrollmentResponse(created), nil
}

// Unenroll marks the calling user's enrollment in the given course as dropped.
func (s *Service) Unenroll(ctx context.Context, callerUserID uint64, courseID uint64) error {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return err
	}
	if course == nil {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	existing, err := s.repo.FindEnrollment(courseID, callerUserID)
	if err != nil {
		return err
	}
	if existing == nil || existing.Status == "dropped" {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "NOT_ENROLLED", Message: "You are not enrolled in this course"}
	}

	_, err = s.repo.UpdateEnrollmentStatus(courseID, callerUserID, "dropped")
	return err
}

// GetEnrollmentsByCourse returns a paginated list of active enrollments for a
// course. Callers must be admin, the assigned teacher, or a principal.
func (s *Service) GetEnrollmentsByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, page, pageSize int) (*PagedEnrollments, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if callerRole != auth.RoleAdmin && callerRole != auth.RolePrincipal && course.TeacherID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only admins, principals, or the assigned teacher can view enrollments"}
	}

	p := Page{Number: page, Size: pageSize}
	enrollments, total, err := s.repo.FindEnrollmentsByCourse(courseID, p)
	if err != nil {
		return nil, err
	}
	return &PagedEnrollments{
		Data:       toEnrollmentResponses(enrollments),
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}

// GetMyEnrollments returns a paginated list of courses the calling user is
// actively enrolled in.
func (s *Service) GetMyEnrollments(ctx context.Context, callerUserID uint64, page, pageSize int) (*PagedEnrollments, error) {
	p := Page{Number: page, Size: pageSize}
	enrollments, total, err := s.repo.FindEnrollmentsByUser(callerUserID, p)
	if err != nil {
		return nil, err
	}
	return &PagedEnrollments{
		Data:       toEnrollmentResponses(enrollments),
		Pagination: buildPaginationMeta(page, pageSize, total),
	}, nil
}

// CreateInvite generates a 10-character invite code and stores it in Redis.
// Only admins, principals, or the assigned teacher may invite to private courses.
func (s *Service) CreateInvite(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, inviteeID uint64) (*CourseInviteResponse, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if callerRole != auth.RoleAdmin && callerRole != auth.RolePrincipal && course.TeacherID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only admins, principals, or the assigned teacher can create invites"}
	}

	if course.Visibility != "private" {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "COURSE_NOT_PRIVATE", Message: "Invites can only be created for private courses"}
	}

	ok, err := s.userExists(ctx, inviteeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "USER_NOT_FOUND", Message: "Invited user not found"}
	}

	enrollment, err := s.repo.FindEnrollment(courseID, inviteeID)
	if err != nil {
		return nil, err
	}
	if enrollment != nil && enrollment.Status == "active" {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ALREADY_ENROLLED", Message: "This user is already enrolled in the course"}
	}

	existing, err := s.repo.FindInviteByCourseAndInvitee(ctx, courseID, inviteeID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case "pending":
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "INVITE_ALREADY_PENDING", Message: "An invite for this user is already pending"}
		case "accepted":
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ALREADY_ENROLLED", Message: "This user has already accepted an invite for this course"}
		case "revoked":
			// Re-invite: update the existing Redis entry back to pending.
			updated, err := s.repo.UpdateInviteStatus(ctx, existing.Code, "pending")
			if err != nil {
				return nil, err
			}
			return toInviteResponse(updated), nil
		}
	}

	inv := &CourseInvite{
		Code:      generateInviteCode(),
		CourseID:  courseID,
		InviteeID: inviteeID,
		InviterID: callerUserID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	if err := s.repo.CreateInvite(ctx, inv); err != nil {
		return nil, err
	}
	return toInviteResponse(inv), nil
}

// AcceptInvite allows the invitee to accept their pending invite, which
// atomically marks the invite as accepted and creates an enrollment.
func (s *Service) AcceptInvite(ctx context.Context, callerUserID uint64, code string) (*EnrollmentResponse, error) {
	invite, err := s.repo.FindInviteByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "INVITE_NOT_FOUND", Message: "Invite not found or expired"}
	}
	if invite.InviteeID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You can only accept your own invites"}
	}
	if invite.Status != "pending" {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "INVITE_NOT_PENDING", Message: "This invite has already been accepted or revoked"}
	}

	existing, err := s.repo.FindEnrollment(invite.CourseID, callerUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == "active" {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ALREADY_ENROLLED", Message: "You are already enrolled in this course"}
	}

	e := &Enrollment{
		CourseID:   invite.CourseID,
		UserID:     callerUserID,
		Status:     "active",
		EnrolledAt: time.Now(),
	}
	_, created, err := s.repo.AcceptInviteAndEnroll(ctx, code, e)
	if err != nil {
		return nil, err
	}
	return toEnrollmentResponse(created), nil
}

// GetInvitesByCourse returns all Redis-stored invites for a course, optionally
// filtered by status. Callers must be admin, principal, or the assigned teacher.
func (s *Service) GetInvitesByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, statusFilter string) ([]*CourseInviteResponse, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}

	if callerRole != auth.RoleAdmin && callerRole != auth.RolePrincipal && course.TeacherID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only admins, principals, or the assigned teacher can view invites"}
	}

	validStatuses := map[string]bool{"pending": true, "accepted": true, "revoked": true}
	if statusFilter != "" && !validStatuses[statusFilter] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "Status must be one of: pending, accepted, revoked"}
	}

	invites, err := s.repo.FindInvitesByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}

	var result []*CourseInviteResponse
	for _, inv := range invites {
		if statusFilter == "" || inv.Status == statusFilter {
			result = append(result, toInviteResponse(inv))
		}
	}
	if result == nil {
		result = []*CourseInviteResponse{}
	}
	return result, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func toEnrollmentResponse(e *Enrollment) *EnrollmentResponse {
	return &EnrollmentResponse{
		ID:         e.ID,
		CourseID:   e.CourseID,
		UserID:     e.UserID,
		Status:     e.Status,
		EnrolledAt: e.EnrolledAt,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

func toEnrollmentResponses(enrollments []*Enrollment) []*EnrollmentResponse {
	result := make([]*EnrollmentResponse, len(enrollments))
	for i, e := range enrollments {
		result[i] = toEnrollmentResponse(e)
	}
	return result
}

func toInviteResponse(inv *CourseInvite) *CourseInviteResponse {
	return &CourseInviteResponse{
		Code:      inv.Code,
		CourseID:  inv.CourseID,
		InviteeID: inv.InviteeID,
		InviterID: inv.InviterID,
		Status:    inv.Status,
		CreatedAt: inv.CreatedAt,
	}
}

func buildPaginationMeta(page, pageSize int, total int64) PaginationMeta {
	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}
	return PaginationMeta{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}
