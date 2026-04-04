package submission

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// AssignmentInfo carries the minimal assignment data needed by the submission
// service without importing the assignment package directly.
type AssignmentInfo struct {
	CourseID uint64
	AuthorID uint64
	DueAt    *time.Time
	Points   uint
	Status   string // draft | published | closed
}

// CourseInfo carries the minimal course data needed for teacher-ownership
// checks without importing the course package directly.
type CourseInfo struct {
	TeacherID uint64
}

// submissionRepository is the interface the Service depends on for data access.
type submissionRepository interface {
	Create(s *AssignmentSubmission) (*AssignmentSubmission, error)
	FindByID(id uint64) (*AssignmentSubmission, error)
	FindByAssignmentAndStudent(assignmentID, studentID uint64) (*AssignmentSubmission, error)
	FindByAssignment(assignmentID uint64) ([]*AssignmentSubmission, error)
	Grade(id uint64, grade uint, feedback string) (*AssignmentSubmission, error)
}

// Service implements the business logic for assignment submissions.
type Service struct {
	repo           submissionRepository
	findAssignment func(ctx context.Context, id uint64) (*AssignmentInfo, error)
	findCourse     func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled     func(ctx context.Context, courseID, userID uint64) (bool, error)
}

// NewService creates a Service wired to the given repository and injected
// dependency functions.
func NewService(
	repo *Repository,
	findAssignment func(ctx context.Context, id uint64) (*AssignmentInfo, error),
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
) *Service {
	return &Service{
		repo:           repo,
		findAssignment: findAssignment,
		findCourse:     findCourse,
		isEnrolled:     isEnrolled,
	}
}

// --- DTOs ---

// SubmissionResponse is the JSON-serialisable representation of an
// AssignmentSubmission returned from all service methods.
type SubmissionResponse struct {
	ID           uint64    `json:"id"`
	AssignmentID uint64    `json:"assignment_id"`
	StudentID    uint64    `json:"student_id"`
	BlobKey      string    `json:"blob_key"`
	FileName     string    `json:"file_name"`
	Status       string    `json:"status"`
	Grade        *uint     `json:"grade"`
	Feedback     string    `json:"feedback"`
	SubmittedAt  time.Time `json:"submitted_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SubmitParams carries the validated inputs for creating a submission.
type SubmitParams struct {
	BlobKey  string
	FileName string
}

// GradeParams carries the validated inputs for grading a submission.
type GradeParams struct {
	Grade    uint
	Feedback string
}

// --- Helpers ---

func toResponse(s *AssignmentSubmission) *SubmissionResponse {
	return &SubmissionResponse{
		ID:           s.ID,
		AssignmentID: s.AssignmentID,
		StudentID:    s.StudentID,
		BlobKey:      s.BlobKey,
		FileName:     s.FileName,
		Status:       s.Status,
		Grade:        s.Grade,
		Feedback:     s.Feedback,
		SubmittedAt:  s.SubmittedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

// resolveAssignment looks up the assignment and returns a typed 404 if absent.
func (s *Service) resolveAssignment(ctx context.Context, assignmentID uint64) (*AssignmentInfo, error) {
	a, err := s.findAssignment(ctx, assignmentID)
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
	return a, nil
}

// resolveAssignmentAndCourse looks up the assignment then its course.
func (s *Service) resolveAssignmentAndCourse(ctx context.Context, assignmentID uint64) (*AssignmentInfo, *CourseInfo, error) {
	a, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return nil, nil, err
	}
	c, err := s.findCourse(ctx, a.CourseID)
	if err != nil {
		return nil, nil, err
	}
	if c == nil {
		return nil, nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "COURSE_NOT_FOUND",
			Message: "Course not found",
		}
	}
	return a, c, nil
}

func isTeacherOrAdmin(callerUserID uint64, callerRole string, course *CourseInfo) bool {
	return callerRole == auth.RoleAdmin || (callerRole == auth.RoleTeacher && course.TeacherID == callerUserID)
}

// --- Service methods ---

// Submit creates a new submission for the calling student.
// The assignment must be published, and the caller must be enrolled (unless admin).
func (s *Service) Submit(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64, p SubmitParams) (*SubmissionResponse, error) {
	a, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	if a.Status != "published" {
		return nil, &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "ASSIGNMENT_NOT_PUBLISHED",
			Message: "Submissions are only accepted for published assignments",
		}
	}

	if callerRole != auth.RoleAdmin {
		enrolled, err := s.isEnrolled(ctx, a.CourseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{
				Status:  http.StatusForbidden,
				Code:    "FORBIDDEN",
				Message: "You must be enrolled in this course to submit",
			}
		}
	}

	status := "submitted"
	if a.DueAt != nil && time.Now().After(*a.DueAt) {
		status = "late"
	}

	sub := &AssignmentSubmission{
		AssignmentID: assignmentID,
		StudentID:    callerUserID,
		BlobKey:      p.BlobKey,
		FileName:     p.FileName,
		Status:       status,
		SubmittedAt:  time.Now(),
	}

	created, err := s.repo.Create(sub)
	if err != nil {
		return nil, err
	}
	if created.ID == 0 {
		return nil, &middleware.APIError{
			Status:  http.StatusConflict,
			Code:    "ALREADY_SUBMITTED",
			Message: "You have already submitted for this assignment",
		}
	}

	return toResponse(created), nil
}

// ListByAssignment returns all submissions for the given assignment.
// Only the course teacher or an admin may call this.
func (s *Service) ListByAssignment(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64) ([]*SubmissionResponse, error) {
	_, course, err := s.resolveAssignmentAndCourse(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can view all submissions",
		}
	}

	rows, err := s.repo.FindByAssignment(assignmentID)
	if err != nil {
		return nil, err
	}

	result := make([]*SubmissionResponse, len(rows))
	for i, r := range rows {
		result[i] = toResponse(r)
	}
	return result, nil
}

// GetMine returns the calling user's own submission for the given assignment.
// Returns 404 when the student has not yet submitted.
func (s *Service) GetMine(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64) (*SubmissionResponse, error) {
	// Verify the assignment exists first.
	if _, err := s.resolveAssignment(ctx, assignmentID); err != nil {
		return nil, err
	}

	sub, err := s.repo.FindByAssignmentAndStudent(assignmentID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "SUBMISSION_NOT_FOUND",
			Message: "You have not submitted for this assignment",
		}
	}

	return toResponse(sub), nil
}

// GradeSubmission sets the grade and optional feedback on a submission.
// Only the course teacher or an admin may call this.
func (s *Service) GradeSubmission(ctx context.Context, callerUserID uint64, callerRole string, assignmentID, submissionID uint64, p GradeParams) (*SubmissionResponse, error) {
	a, course, err := s.resolveAssignmentAndCourse(ctx, assignmentID)
	if err != nil {
		return nil, err
	}

	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can grade submissions",
		}
	}

	// Validate grade range only when the assignment has a non-zero point value.
	if a.Points > 0 && p.Grade > a.Points {
		return nil, &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "INVALID_GRADE",
			Message: "Grade exceeds the maximum points for this assignment",
		}
	}

	graded, err := s.repo.Grade(submissionID, p.Grade, p.Feedback)
	if err != nil {
		return nil, err
	}
	if graded == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "SUBMISSION_NOT_FOUND",
			Message: "Submission not found",
		}
	}

	return toResponse(graded), nil
}
