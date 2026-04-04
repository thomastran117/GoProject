package grade

import (
	"context"
	"net/http"
	"sort"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

// CourseInfo carries the minimal course data needed for ownership checks
// without importing the course package directly.
type CourseInfo struct {
	TeacherID uint64
}

// ItemInfo carries the minimal data needed to validate that a referenced
// assignment, quiz, test, or exam belongs to the expected course.
type ItemInfo struct {
	CourseID uint64
}

// gradeRepository is the interface the Service depends on for data access.
type gradeRepository interface {
	Create(ctx context.Context, g *Grade) (*Grade, error)
	FindByID(ctx context.Context, id uint64) (*Grade, error)
	FindByCourse(ctx context.Context, courseID uint64) ([]*Grade, error)
	FindByCourseAndStudent(ctx context.Context, courseID, studentID uint64) ([]*Grade, error)
	Update(ctx context.Context, id uint64, fields map[string]any) (*Grade, error)
	Delete(ctx context.Context, id uint64) (bool, error)
}

// Service implements the business logic for grades.
type Service struct {
	repo           gradeRepository
	findCourse     func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled     func(ctx context.Context, courseID, userID uint64) (bool, error)
	findAssignment func(ctx context.Context, id uint64) (*ItemInfo, error)
	findQuiz       func(ctx context.Context, id uint64) (*ItemInfo, error)
	findTest       func(ctx context.Context, id uint64) (*ItemInfo, error)
	findExam       func(ctx context.Context, id uint64) (*ItemInfo, error)
}

// NewService creates a Service wired to the given repository and injected
// dependency functions.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
	findAssignment func(ctx context.Context, id uint64) (*ItemInfo, error),
	findQuiz func(ctx context.Context, id uint64) (*ItemInfo, error),
	findTest func(ctx context.Context, id uint64) (*ItemInfo, error),
	findExam func(ctx context.Context, id uint64) (*ItemInfo, error),
) *Service {
	return &Service{
		repo:           repo,
		findCourse:     findCourse,
		isEnrolled:     isEnrolled,
		findAssignment: findAssignment,
		findQuiz:       findQuiz,
		findTest:       findTest,
		findExam:       findExam,
	}
}

// --- DTOs ---

// GradeResponse is the JSON-serialisable representation of a single grade.
// Type and ReferenceID are derived from whichever FK column is non-nil.
type GradeResponse struct {
	ID          uint64    `json:"id"`
	CourseID    uint64    `json:"course_id"`
	StudentID   uint64    `json:"student_id"`
	Type        string    `json:"type"`         // assignment | quiz | test | exam
	ReferenceID uint64    `json:"reference_id"` // value of the non-nil FK
	Title       string    `json:"title"`
	Score       float64   `json:"score"`
	MaxScore    float64   `json:"max_score"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StudentGradesResponse groups all grades for one student with a computed
// final_grade. Used in the teacher/admin ListAll response.
type StudentGradesResponse struct {
	StudentID  uint64           `json:"student_id"`
	Grades     []*GradeResponse `json:"grades"`
	FinalGrade *float64         `json:"final_grade"`
}

// MyGradesResponse is returned by the /mine endpoint.
type MyGradesResponse struct {
	Grades     []*GradeResponse `json:"grades"`
	FinalGrade *float64         `json:"final_grade"`
}

// --- Params ---

// CreateGradeParams carries the validated inputs for creating a grade.
type CreateGradeParams struct {
	StudentID    uint64
	AssignmentID *uint64
	QuizID       *uint64
	TestID       *uint64
	ExamID       *uint64
	Title        string
	Score        float64
	MaxScore     float64
}

// UpdateGradeParams carries the validated inputs for updating a grade.
// Nil pointer fields are not updated. FK columns are immutable after creation.
type UpdateGradeParams struct {
	Title    *string
	Score    *float64
	MaxScore *float64
}

// --- Helpers ---

// gradeType derives the type string from which FK column is non-nil.
func gradeType(g *Grade) string {
	if g.AssignmentID != nil {
		return "assignment"
	}
	if g.QuizID != nil {
		return "quiz"
	}
	if g.TestID != nil {
		return "test"
	}
	return "exam"
}

// gradeReferenceID returns the value of the non-nil FK column.
func gradeReferenceID(g *Grade) uint64 {
	if g.AssignmentID != nil {
		return *g.AssignmentID
	}
	if g.QuizID != nil {
		return *g.QuizID
	}
	if g.TestID != nil {
		return *g.TestID
	}
	return *g.ExamID
}

// toResponse maps a *Grade DB row to a *GradeResponse DTO.
func toResponse(g *Grade) *GradeResponse {
	return &GradeResponse{
		ID:          g.ID,
		CourseID:    g.CourseID,
		StudentID:   g.StudentID,
		Type:        gradeType(g),
		ReferenceID: gradeReferenceID(g),
		Title:       g.Title,
		Score:       g.Score,
		MaxScore:    g.MaxScore,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// finalGrade computes sum(score)/sum(max_score)*100 from pre-summed values.
// Returns nil when sumMax is zero (no grades or all max scores are zero).
func finalGrade(sumScore, sumMax float64) *float64 {
	if sumMax == 0 {
		return nil
	}
	v := (sumScore / sumMax) * 100
	return &v
}

// computeFinalGrade returns the final grade percentage for a []*Grade slice.
func computeFinalGrade(grades []*Grade) *float64 {
	var sumScore, sumMax float64
	for _, g := range grades {
		sumScore += g.Score
		sumMax += g.MaxScore
	}
	return finalGrade(sumScore, sumMax)
}

// computeFinalGradeFromResponses returns the final grade percentage for a
// []*GradeResponse slice, avoiding reconstruction of Grade objects.
func computeFinalGradeFromResponses(grades []*GradeResponse) *float64 {
	var sumScore, sumMax float64
	for _, g := range grades {
		sumScore += g.Score
		sumMax += g.MaxScore
	}
	return finalGrade(sumScore, sumMax)
}

// groupByStudent groups a flat []*Grade slice into []*StudentGradesResponse,
// preserving the student_id ASC, created_at ASC order returned by the repository.
func groupByStudent(grades []*Grade) []*StudentGradesResponse {
	var result []*StudentGradesResponse
	index := map[uint64]int{}

	for _, g := range grades {
		i, ok := index[g.StudentID]
		if !ok {
			i = len(result)
			index[g.StudentID] = i
			result = append(result, &StudentGradesResponse{
				StudentID: g.StudentID,
				Grades:    []*GradeResponse{},
			})
		}
		result[i].Grades = append(result[i].Grades, toResponse(g))
	}

	for _, sg := range result {
		sg.FinalGrade = computeFinalGradeFromResponses(sg.Grades)
	}

	// Sort by StudentID so the output order is deterministic regardless of any
	// future change to the upstream query ordering.
	sort.Slice(result, func(i, j int) bool {
		return result[i].StudentID < result[j].StudentID
	})
	return result
}

func (s *Service) resolveCourse(ctx context.Context, courseID uint64) (*CourseInfo, error) {
	c, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "COURSE_NOT_FOUND",
			Message: "Course not found",
		}
	}
	return c, nil
}

func isTeacherOrAdmin(callerUserID uint64, callerRole string, course *CourseInfo) bool {
	return callerRole == auth.RoleAdmin || (callerRole == auth.RoleTeacher && course.TeacherID == callerUserID)
}

// countFKs returns how many of the four FK fields are non-nil.
func countFKs(p *CreateGradeParams) int {
	n := 0
	if p.AssignmentID != nil {
		n++
	}
	if p.QuizID != nil {
		n++
	}
	if p.TestID != nil {
		n++
	}
	if p.ExamID != nil {
		n++
	}
	return n
}

// resolveReference looks up the referenced item and verifies it belongs to
// courseID. Called only after countFKs confirms exactly one FK is set.
func (s *Service) resolveReference(ctx context.Context, courseID uint64, p *CreateGradeParams) error {
	var item *ItemInfo
	var err error

	switch {
	case p.AssignmentID != nil:
		item, err = s.findAssignment(ctx, *p.AssignmentID)
	case p.QuizID != nil:
		item, err = s.findQuiz(ctx, *p.QuizID)
	case p.TestID != nil:
		item, err = s.findTest(ctx, *p.TestID)
	case p.ExamID != nil:
		item, err = s.findExam(ctx, *p.ExamID)
	default:
		// Unreachable: countFKs == 1 is enforced before this call.
		return &middleware.APIError{
			Status:  http.StatusInternalServerError,
			Code:    "INTERNAL_ERROR",
			Message: "No reference ID provided",
		}
	}

	if err != nil {
		return err
	}
	if item == nil {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "REFERENCE_NOT_FOUND",
			Message: "The referenced assignment, quiz, test, or exam was not found",
		}
	}
	if item.CourseID != courseID {
		return &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "REFERENCE_COURSE_MISMATCH",
			Message: "The referenced item does not belong to this course",
		}
	}
	return nil
}

// --- Service methods ---

// CreateGrade creates a grade for a student in a course.
// Only the course teacher or an admin may call this.
func (s *Service) CreateGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateGradeParams) (*GradeResponse, error) {
	course, err := s.resolveCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can create grades",
		}
	}

	if countFKs(&p) != 1 {
		return nil, &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "INVALID_REFERENCE",
			Message: "Exactly one of assignment_id, quiz_id, test_id, exam_id must be provided",
		}
	}

	if err := s.resolveReference(ctx, courseID, &p); err != nil {
		return nil, err
	}

	if p.MaxScore == 0 {
		p.MaxScore = 100
	}
	if p.Score > p.MaxScore {
		return nil, &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "INVALID_SCORE",
			Message: "Score cannot exceed MaxScore",
		}
	}

	g := &Grade{
		CourseID:     courseID,
		StudentID:    p.StudentID,
		AssignmentID: p.AssignmentID,
		QuizID:       p.QuizID,
		TestID:       p.TestID,
		ExamID:       p.ExamID,
		Title:        p.Title,
		Score:        p.Score,
		MaxScore:     p.MaxScore,
	}
	created, err := s.repo.Create(ctx, g)
	if err != nil {
		return nil, err
	}
	return toResponse(created), nil
}

// ListAll returns all grades for all students in the course, grouped by
// student with a computed final_grade.
// Only the course teacher or an admin may call this.
func (s *Service) ListAll(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*StudentGradesResponse, error) {
	course, err := s.resolveCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can view all grades",
		}
	}

	rows, err := s.repo.FindByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	return groupByStudent(rows), nil
}

// GetMine returns all grades for the calling user in the course, plus a
// computed final_grade. Students must be actively enrolled; teachers and
// admins bypass this check.
func (s *Service) GetMine(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) (*MyGradesResponse, error) {
	// resolveCourse guarantees a non-nil CourseInfo on success; reuse it for
	// the teacher-ownership check to avoid a redundant findCourse call.
	course, err := s.resolveCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}

	if callerRole != auth.RoleAdmin {
		if callerRole != auth.RoleTeacher || course.TeacherID != callerUserID {
			enrolled, err := s.isEnrolled(ctx, courseID, callerUserID)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				return nil, &middleware.APIError{
					Status:  http.StatusForbidden,
					Code:    "FORBIDDEN",
					Message: "You must be enrolled in this course to view your grades",
				}
			}
		}
	}

	rows, err := s.repo.FindByCourseAndStudent(ctx, courseID, callerUserID)
	if err != nil {
		return nil, err
	}

	data := make([]*GradeResponse, len(rows))
	for i, g := range rows {
		data[i] = toResponse(g)
	}
	return &MyGradesResponse{
		Grades:     data,
		FinalGrade: computeFinalGrade(rows),
	}, nil
}

// UpdateGrade updates mutable fields on a grade entry.
// Only the course teacher or an admin may call this.
func (s *Service) UpdateGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID, gradeID uint64, p UpdateGradeParams) (*GradeResponse, error) {
	course, err := s.resolveCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return nil, &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can update grades",
		}
	}

	existing, err := s.repo.FindByID(ctx, gradeID)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.CourseID != courseID {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "GRADE_NOT_FOUND",
			Message: "Grade not found",
		}
	}

	// Merge incoming fields onto existing values before validating.
	mergedScore := existing.Score
	mergedMax := existing.MaxScore
	if p.Score != nil {
		mergedScore = *p.Score
	}
	if p.MaxScore != nil {
		mergedMax = *p.MaxScore
	}
	if mergedScore > mergedMax {
		return nil, &middleware.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "INVALID_SCORE",
			Message: "Score cannot exceed MaxScore",
		}
	}

	fields := map[string]any{
		"score":     mergedScore,
		"max_score": mergedMax,
	}
	if p.Title != nil {
		fields["title"] = *p.Title
	}

	updated, err := s.repo.Update(ctx, gradeID, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "GRADE_NOT_FOUND",
			Message: "Grade not found",
		}
	}
	return toResponse(updated), nil
}

// DeleteGrade removes a grade entry.
// Only the course teacher or an admin may call this.
func (s *Service) DeleteGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID, gradeID uint64) error {
	course, err := s.resolveCourse(ctx, courseID)
	if err != nil {
		return err
	}
	if !isTeacherOrAdmin(callerUserID, callerRole, course) {
		return &middleware.APIError{
			Status:  http.StatusForbidden,
			Code:    "FORBIDDEN",
			Message: "Only the course teacher or an admin can delete grades",
		}
	}

	existing, err := s.repo.FindByID(ctx, gradeID)
	if err != nil {
		return err
	}
	if existing == nil || existing.CourseID != courseID {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "GRADE_NOT_FOUND",
			Message: "Grade not found",
		}
	}

	_, err = s.repo.Delete(ctx, gradeID)
	return err
}
