package quiz

import (
	"context"
	"net/http"
	"strings"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
	"backend/internal/features/grade"
)

// CourseInfo carries the minimal course data needed for ownership checks.
type CourseInfo struct {
	TeacherID uint64
}

// quizRepository is the data-access interface the Service depends on.
type quizRepository interface {
	Create(ctx context.Context, q *Quiz) (*Quiz, error)
	FindByID(ctx context.Context, id uint64) (*Quiz, error)
	FindByCourse(ctx context.Context, courseID uint64) ([]*Quiz, error)
	Update(ctx context.Context, id uint64, fields map[string]any) (*Quiz, error)
	Delete(ctx context.Context, id uint64) (bool, error)

	CreateQuestion(ctx context.Context, q *QuizQuestion) (*QuizQuestion, error)
	FindQuestionByID(ctx context.Context, id uint64) (*QuizQuestion, error)
	FindQuestionsWithChoices(ctx context.Context, quizID uint64) ([]*QuizQuestion, error)
	UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*QuizQuestion, error)
	DeleteQuestion(ctx context.Context, id uint64) (bool, error)
	DeleteChoicesByQuestion(ctx context.Context, questionID uint64) error
	CountQuestionsByQuiz(ctx context.Context, quizID uint64) (int64, error)

	CreateChoice(ctx context.Context, c *QuizChoice) (*QuizChoice, error)
	FindChoiceByID(ctx context.Context, id uint64) (*QuizChoice, error)
	FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*QuizChoice, error)
	CountChoicesByQuestion(ctx context.Context, questionID uint64) (int64, error)
	UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*QuizChoice, error)
	DeleteChoice(ctx context.Context, id uint64) (bool, error)

	CreateSubmission(ctx context.Context, s *QuizSubmission) (*QuizSubmission, error)
	FindSubmissionByID(ctx context.Context, id uint64) (*QuizSubmission, error)
	FindSubmissionByQuizAndStudent(ctx context.Context, quizID, studentID uint64) (*QuizSubmission, error)
	FindSubmissionsByQuiz(ctx context.Context, quizID uint64) ([]*QuizSubmission, error)
	CountActiveSubmissions(ctx context.Context, quizID uint64) (int64, error)
	UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*QuizSubmission, error)

	UpsertAnswer(ctx context.Context, a *QuizAnswer) (*QuizAnswer, error)
	FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*QuizAnswer, error)
	UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*QuizAnswer, error)
}

// Service implements the business logic for quizzes.
type Service struct {
	repo        quizRepository
	findCourse  func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled  func(ctx context.Context, courseID, userID uint64) (bool, error)
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error)
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error)
	findGrade   func(ctx context.Context, quizID, studentID uint64) (*grade.Grade, error)
}

// NewService creates a Service with all required dependencies injected.
func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error),
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error),
	findGrade func(ctx context.Context, quizID, studentID uint64) (*grade.Grade, error),
) *Service {
	return &Service{
		repo:        repo,
		findCourse:  findCourse,
		isEnrolled:  isEnrolled,
		createGrade: createGrade,
		updateGrade: updateGrade,
		findGrade:   findGrade,
	}
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type StudentChoiceResponse struct {
	ID        uint64 `json:"id"`
	SortOrder int    `json:"sort_order"`
	Text      string `json:"text"`
}

type ChoiceResponse struct {
	ID        uint64 `json:"id"`
	SortOrder int    `json:"sort_order"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"is_correct"`
}

type StudentQuestionResponse struct {
	ID           uint64                   `json:"id"`
	SortOrder    int                      `json:"sort_order"`
	QuestionType string                   `json:"question_type"`
	Text         string                   `json:"text"`
	ImageBlobKey string                   `json:"image_blob_key,omitempty"`
	Weight       float64                  `json:"weight"`
	Choices      []*StudentChoiceResponse `json:"choices,omitempty"`
}

type QuestionResponse struct {
	ID            uint64            `json:"id"`
	SortOrder     int               `json:"sort_order"`
	QuestionType  string            `json:"question_type"`
	Text          string            `json:"text"`
	ImageBlobKey  string            `json:"image_blob_key,omitempty"`
	Weight        float64           `json:"weight"`
	CorrectAnswer string            `json:"correct_answer,omitempty"`
	Choices       []*ChoiceResponse `json:"choices,omitempty"`
}

type QuizResponse struct {
	ID          uint64    `json:"id"`
	CourseID    uint64    `json:"course_id"`
	AuthorID    uint64    `json:"author_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	QuizType    string    `json:"quiz_type"`
	ExternalURL string    `json:"external_url,omitempty"`
	Status      string    `json:"status"`
	DueAt       *time.Time `json:"due_at"`
	TotalWeight float64   `json:"total_weight"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AnswerResponse struct {
	ID            uint64   `json:"id"`
	QuestionID    uint64   `json:"question_id"`
	AnswerText    string   `json:"answer_text"`
	PointsAwarded *float64 `json:"points_awarded"`
	NeedsReview   bool     `json:"needs_review"`
}

type SubmissionResponse struct {
	ID          uint64           `json:"id"`
	QuizID      uint64           `json:"quiz_id"`
	StudentID   uint64           `json:"student_id"`
	Status      string           `json:"status"`
	StartedAt   time.Time        `json:"started_at"`
	SubmittedAt *time.Time       `json:"submitted_at"`
	Score       *float64         `json:"score"`
	MaxScore    float64          `json:"max_score"`
	GradeID     *uint64          `json:"grade_id"`
	NeedsReview int              `json:"needs_review"`
	Answers     []*AnswerResponse `json:"answers,omitempty"`
}

// StartQuizResponse bundles the submission with questions (correct answers hidden).
type StartQuizResponse struct {
	Submission *SubmissionResponse        `json:"submission"`
	Questions  []*StudentQuestionResponse `json:"questions"`
}

// ─── Params ───────────────────────────────────────────────────────────────────

type CreateQuizParams struct {
	Title       string
	Description string
	QuizType    string
	ExternalURL string
	Status      string
	DueAt       *time.Time
}

type UpdateQuizParams struct {
	Title       string
	Description string
	ExternalURL string
	Status      string
	DueAt       *time.Time
}

type CreateQuestionParams struct {
	SortOrder    int
	QuestionType string
	Text         string
	ImageBlobKey string
	Weight       float64
	CorrectAnswer string
}

type UpdateQuestionParams struct {
	SortOrder    int
	QuestionType string
	Text         string
	ImageBlobKey string
	Weight       float64
	CorrectAnswer string
}

type CreateChoiceParams struct {
	SortOrder int
	Text      string
	IsCorrect bool
}

type UpdateChoiceParams struct {
	SortOrder int
	Text      string
	IsCorrect bool
}

type GradeAnswerParams struct {
	PointsAwarded float64
}

// ─── Validation ───────────────────────────────────────────────────────────────

var validQuizTypes = map[string]bool{"link": true, "in_house": true}
var validQuestionTypes = map[string]bool{
	"multiple_choice": true,
	"short_answer":    true,
	"fill_in_blank":   true,
	"long_answer":     true,
}
var validStatuses = map[string]bool{"draft": true, "published": true, "closed": true}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toQuizResponse(q *Quiz) *QuizResponse {
	return &QuizResponse{
		ID:          q.ID,
		CourseID:    q.CourseID,
		AuthorID:    q.AuthorID,
		Title:       q.Title,
		Description: q.Description,
		QuizType:    q.QuizType,
		ExternalURL: q.ExternalURL,
		Status:      q.Status,
		DueAt:       q.DueAt,
		CreatedAt:   q.CreatedAt,
		UpdatedAt:   q.UpdatedAt,
	}
}

func toQuestionResponse(q *QuizQuestion) *QuestionResponse {
	resp := &QuestionResponse{
		ID:            q.ID,
		SortOrder:     q.SortOrder,
		QuestionType:  q.QuestionType,
		Text:          q.Text,
		ImageBlobKey:  q.ImageBlobKey,
		Weight:        q.Weight,
		CorrectAnswer: q.CorrectAnswer,
	}
	for _, c := range q.Choices {
		resp.Choices = append(resp.Choices, &ChoiceResponse{
			ID:        c.ID,
			SortOrder: c.SortOrder,
			Text:      c.Text,
			IsCorrect: c.IsCorrect,
		})
	}
	return resp
}

func toStudentQuestionResponse(q *QuizQuestion) *StudentQuestionResponse {
	resp := &StudentQuestionResponse{
		ID:           q.ID,
		SortOrder:    q.SortOrder,
		QuestionType: q.QuestionType,
		Text:         q.Text,
		ImageBlobKey: q.ImageBlobKey,
		Weight:       q.Weight,
	}
	if q.QuestionType == "multiple_choice" {
		for _, c := range q.Choices {
			resp.Choices = append(resp.Choices, &StudentChoiceResponse{
				ID:        c.ID,
				SortOrder: c.SortOrder,
				Text:      c.Text,
			})
		}
	}
	return resp
}

func toAnswerResponse(a *QuizAnswer) *AnswerResponse {
	return &AnswerResponse{
		ID:            a.ID,
		QuestionID:    a.QuestionID,
		AnswerText:    a.AnswerText,
		PointsAwarded: a.PointsAwarded,
		NeedsReview:   a.NeedsReview,
	}
}

func toSubmissionResponse(s *QuizSubmission, answers []*QuizAnswer) *SubmissionResponse {
	resp := &SubmissionResponse{
		ID:          s.ID,
		QuizID:      s.QuizID,
		StudentID:   s.StudentID,
		Status:      s.Status,
		StartedAt:   s.StartedAt,
		SubmittedAt: s.SubmittedAt,
		Score:       s.Score,
		MaxScore:    s.MaxScore,
		GradeID:     s.GradeID,
	}
	needsReview := 0
	for _, a := range answers {
		if a.NeedsReview {
			needsReview++
		}
		resp.Answers = append(resp.Answers, toAnswerResponse(a))
	}
	resp.NeedsReview = needsReview
	return resp
}

func (s *Service) requireCourseTeacherOrAdmin(ctx context.Context, courseID, callerUserID uint64, callerRole string) (*CourseInfo, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}
	if callerRole != auth.RoleAdmin {
		if callerRole != auth.RoleTeacher || course.TeacherID != callerUserID {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can manage quizzes"}
		}
	}
	return course, nil
}

func (s *Service) requireQuizAuthorOrAdmin(ctx context.Context, quizID, callerUserID uint64, callerRole string) (*Quiz, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if callerRole != auth.RoleAdmin && q.AuthorID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the quiz author or an admin can perform this action"}
	}
	return q, nil
}

func (s *Service) requireInHouseQuiz(q *Quiz) error {
	if q.QuizType == "link" {
		return &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "NOT_APPLICABLE", Message: "This operation is not applicable to link-type quizzes"}
	}
	return nil
}

func sumWeights(questions []*QuizQuestion) float64 {
	total := 0.0
	for _, q := range questions {
		total += q.Weight
	}
	return total
}

// ─── Quiz CRUD ────────────────────────────────────────────────────────────────

// CreateQuiz creates a new quiz in the given course.
func (s *Service) CreateQuiz(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateQuizParams) (*QuizResponse, error) {
	if _, err := s.requireCourseTeacherOrAdmin(ctx, courseID, callerUserID, callerRole); err != nil {
		return nil, err
	}
	if !validQuizTypes[p.QuizType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUIZ_TYPE", Message: "quiz_type must be 'link' or 'in_house'"}
	}
	if p.QuizType == "link" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url is required for link-type quizzes"}
	}
	if p.QuizType == "in_house" && strings.TrimSpace(p.ExternalURL) != "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_FORBIDDEN", Message: "external_url must not be set for in_house quizzes"}
	}
	status := p.Status
	if status == "" {
		status = "draft"
	}
	if !validStatuses[status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	q := &Quiz{
		CourseID:    courseID,
		AuthorID:    callerUserID,
		Title:       p.Title,
		Description: p.Description,
		QuizType:    p.QuizType,
		ExternalURL: p.ExternalURL,
		Status:      status,
		DueAt:       p.DueAt,
	}
	created, err := s.repo.Create(ctx, q)
	if err != nil {
		return nil, err
	}
	return toQuizResponse(created), nil
}

// GetQuiz returns a quiz visible to the caller.
func (s *Service) GetQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error) {
	q, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if callerRole != auth.RoleAdmin {
		course, err := s.findCourse(ctx, q.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil || course.TeacherID != callerUserID {
			enrolled, err := s.isEnrolled(ctx, q.CourseID, callerUserID)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its quizzes"}
			}
		}
	}
	resp := toQuizResponse(q)
	questions, err := s.repo.FindQuestionsWithChoices(ctx, id)
	if err != nil {
		return nil, err
	}
	resp.TotalWeight = sumWeights(questions)
	return resp, nil
}

// ListByCourse returns all quizzes for a course.
func (s *Service) ListByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*QuizResponse, error) {
	course, err := s.findCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "COURSE_NOT_FOUND", Message: "Course not found"}
	}
	if callerRole != auth.RoleAdmin && course.TeacherID != callerUserID {
		enrolled, err := s.isEnrolled(ctx, courseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its quizzes"}
		}
	}
	quizzes, err := s.repo.FindByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make([]*QuizResponse, len(quizzes))
	for i, q := range quizzes {
		out[i] = toQuizResponse(q)
	}
	return out, nil
}

// UpdateQuiz modifies an existing quiz.
func (s *Service) UpdateQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64, p UpdateQuizParams) (*QuizResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if p.Status != "" && !validStatuses[p.Status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	if q.QuizType == "link" && p.ExternalURL != "" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url cannot be cleared for a link-type quiz"}
	}
	fields := map[string]any{
		"title":        p.Title,
		"description":  p.Description,
		"external_url": p.ExternalURL,
		"due_at":       p.DueAt,
	}
	if p.Status != "" {
		fields["status"] = p.Status
	}
	updated, err := s.repo.Update(ctx, id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	return toQuizResponse(updated), nil
}

// DeleteQuiz removes a quiz and all its questions, choices, and submissions.
func (s *Service) DeleteQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) error {
	if _, err := s.requireQuizAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return err
	}
	active, err := s.repo.CountActiveSubmissions(ctx, id)
	if err != nil {
		return err
	}
	if active > 0 {
		return &middleware.APIError{Status: http.StatusConflict, Code: "QUIZ_HAS_SUBMISSIONS", Message: "Cannot delete a quiz with active submissions"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, id)
	if err != nil {
		return err
	}
	for _, q := range questions {
		if err := s.repo.DeleteChoicesByQuestion(ctx, q.ID); err != nil {
			return err
		}
		if _, err := s.repo.DeleteQuestion(ctx, q.ID); err != nil {
			return err
		}
	}
	_, err = s.repo.Delete(ctx, id)
	return err
}

// PublishQuiz sets a quiz status to published.
func (s *Service) PublishQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if q.QuizType == "in_house" {
		count, err := s.repo.CountQuestionsByQuiz(ctx, id)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "QUIZ_HAS_NO_QUESTIONS", Message: "Cannot publish an in_house quiz with no questions"}
		}
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "published"})
	if err != nil {
		return nil, err
	}
	return toQuizResponse(updated), nil
}

// CloseQuiz sets a quiz status to closed.
func (s *Service) CloseQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error) {
	if _, err := s.requireQuizAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "closed"})
	if err != nil {
		return nil, err
	}
	return toQuizResponse(updated), nil
}

// ─── Question management ──────────────────────────────────────────────────────

// ListQuestions returns all questions for a quiz with their choices.
func (s *Service) ListQuestions(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) ([]*QuestionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if callerRole != auth.RoleAdmin && q.AuthorID != callerUserID {
		enrolled, err := s.isEnrolled(ctx, q.CourseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to view quiz questions"}
		}
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, quizID)
	if err != nil {
		return nil, err
	}
	out := make([]*QuestionResponse, len(questions))
	for i, qq := range questions {
		out[i] = toQuestionResponse(qq)
	}
	return out, nil
}

// CreateQuestion adds a question to a quiz.
func (s *Service) CreateQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64, p CreateQuestionParams) (*QuestionResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	if !validQuestionTypes[p.QuestionType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUESTION_TYPE", Message: "question_type must be one of: multiple_choice, short_answer, fill_in_blank, long_answer"}
	}
	if p.Weight <= 0 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_WEIGHT", Message: "weight must be greater than 0"}
	}
	question := &QuizQuestion{
		QuizID:        quizID,
		SortOrder:     p.SortOrder,
		QuestionType:  p.QuestionType,
		Text:          p.Text,
		ImageBlobKey:  p.ImageBlobKey,
		Weight:        p.Weight,
		CorrectAnswer: p.CorrectAnswer,
	}
	created, err := s.repo.CreateQuestion(ctx, question)
	if err != nil {
		return nil, err
	}
	return toQuestionResponse(created), nil
}

// UpdateQuestion modifies an existing question.
func (s *Service) UpdateQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64, p UpdateQuestionParams) (*QuestionResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if p.QuestionType != "" && !validQuestionTypes[p.QuestionType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUESTION_TYPE", Message: "question_type must be one of: multiple_choice, short_answer, fill_in_blank, long_answer"}
	}
	if p.Weight <= 0 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_WEIGHT", Message: "weight must be greater than 0"}
	}
	fields := map[string]any{
		"sort_order":     p.SortOrder,
		"question_type":  p.QuestionType,
		"text":           p.Text,
		"image_blob_key": p.ImageBlobKey,
		"weight":         p.Weight,
		"correct_answer": p.CorrectAnswer,
	}
	updated, err := s.repo.UpdateQuestion(ctx, questionID, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	// Re-fetch with choices
	questions, err := s.repo.FindQuestionsWithChoices(ctx, quizID)
	if err != nil {
		return nil, err
	}
	for _, qq := range questions {
		if qq.ID == questionID {
			return toQuestionResponse(qq), nil
		}
	}
	return toQuestionResponse(updated), nil
}

// DeleteQuestion removes a question and all its choices.
func (s *Service) DeleteQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64) error {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.QuizID != quizID {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if err := s.repo.DeleteChoicesByQuestion(ctx, questionID); err != nil {
		return err
	}
	_, err = s.repo.DeleteQuestion(ctx, questionID)
	return err
}

// ─── Choice management ────────────────────────────────────────────────────────

// CreateChoice adds a choice to a multiple_choice question.
func (s *Service) CreateChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64, p CreateChoiceParams) (*ChoiceResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if question.QuestionType != "multiple_choice" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_QUESTION_TYPE", Message: "Choices can only be added to multiple_choice questions"}
	}
	count, err := s.repo.CountChoicesByQuestion(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if count >= 15 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_CHOICE_COUNT", Message: "A question can have at most 15 choices"}
	}
	choice := &QuizChoice{
		QuestionID: questionID,
		SortOrder:  p.SortOrder,
		Text:       p.Text,
		IsCorrect:  p.IsCorrect,
	}
	created, err := s.repo.CreateChoice(ctx, choice)
	if err != nil {
		return nil, err
	}
	return &ChoiceResponse{ID: created.ID, SortOrder: created.SortOrder, Text: created.Text, IsCorrect: created.IsCorrect}, nil
}

// UpdateChoice modifies a choice.
func (s *Service) UpdateChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID, choiceID uint64, p UpdateChoiceParams) (*ChoiceResponse, error) {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	choice, err := s.repo.FindChoiceByID(ctx, choiceID)
	if err != nil {
		return nil, err
	}
	if choice == nil || choice.QuestionID != questionID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "CHOICE_NOT_FOUND", Message: "Choice not found"}
	}
	updated, err := s.repo.UpdateChoice(ctx, choiceID, map[string]any{
		"sort_order": p.SortOrder,
		"text":       p.Text,
		"is_correct": p.IsCorrect,
	})
	if err != nil {
		return nil, err
	}
	return &ChoiceResponse{ID: updated.ID, SortOrder: updated.SortOrder, Text: updated.Text, IsCorrect: updated.IsCorrect}, nil
}

// DeleteChoice removes a choice.
func (s *Service) DeleteChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID, choiceID uint64) error {
	q, err := s.requireQuizAuthorOrAdmin(ctx, quizID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.QuizID != quizID {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	choice, err := s.repo.FindChoiceByID(ctx, choiceID)
	if err != nil {
		return err
	}
	if choice == nil || choice.QuestionID != questionID {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "CHOICE_NOT_FOUND", Message: "Choice not found"}
	}
	_, err = s.repo.DeleteChoice(ctx, choiceID)
	return err
}

// ─── Student workflow ─────────────────────────────────────────────────────────

// StartQuiz begins a student's quiz attempt.
func (s *Service) StartQuiz(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) (*StartQuizResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	if q.Status != "published" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "QUIZ_NOT_PUBLISHED", Message: "This quiz is not published"}
	}
	if q.DueAt != nil && time.Now().After(*q.DueAt) {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "QUIZ_PAST_DUE", Message: "This quiz is past its due date"}
	}
	enrolled, err := s.isEnrolled(ctx, q.CourseID, callerUserID)
	if err != nil {
		return nil, err
	}
	if !enrolled {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to take this quiz"}
	}

	sub := &QuizSubmission{
		QuizID:    quizID,
		StudentID: callerUserID,
		StartedAt: time.Now(),
	}
	created, err := s.repo.CreateSubmission(ctx, sub)
	if err != nil {
		return nil, err
	}
	if created.ID == 0 {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "QUIZ_ALREADY_STARTED", Message: "You have already started this quiz"}
	}

	questions, err := s.repo.FindQuestionsWithChoices(ctx, quizID)
	if err != nil {
		return nil, err
	}
	studentQuestions := make([]*StudentQuestionResponse, len(questions))
	for i, qq := range questions {
		studentQuestions[i] = toStudentQuestionResponse(qq)
	}

	return &StartQuizResponse{
		Submission: toSubmissionResponse(created, nil),
		Questions:  studentQuestions,
	}, nil
}

// SaveAnswer saves or updates a student's answer for one question.
func (s *Service) SaveAnswer(ctx context.Context, callerUserID uint64, quizID, questionID uint64, answerText string) (*AnswerResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByQuizAndStudent(ctx, quizID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this quiz"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "QUIZ_ALREADY_SUBMITTED", Message: "This quiz has already been submitted"}
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	answer, err := s.repo.UpsertAnswer(ctx, &QuizAnswer{
		SubmissionID: sub.ID,
		QuestionID:   questionID,
		AnswerText:   answerText,
	})
	if err != nil {
		return nil, err
	}
	return toAnswerResponse(answer), nil
}

// SubmitQuiz finalises a student's submission and auto-grades where possible.
func (s *Service) SubmitQuiz(ctx context.Context, callerUserID uint64, quizID uint64) (*SubmissionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByQuizAndStudent(ctx, quizID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this quiz"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "QUIZ_NOT_IN_PROGRESS", Message: "This submission is not in progress"}
	}

	questions, err := s.repo.FindQuestionsWithChoices(ctx, quizID)
	if err != nil {
		return nil, err
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}

	// Index answers by question ID
	answerMap := make(map[uint64]*QuizAnswer, len(answers))
	for _, a := range answers {
		answerMap[a.QuestionID] = a
	}

	maxScore := sumWeights(questions)
	score := 0.0

	for _, question := range questions {
		answer, exists := answerMap[question.ID]
		if !exists {
			// No answer provided — create a blank one so grading is recorded
			blank := &QuizAnswer{
				SubmissionID: sub.ID,
				QuestionID:   question.ID,
				AnswerText:   "",
			}
			created, err := s.repo.UpsertAnswer(ctx, blank)
			if err != nil {
				return nil, err
			}
			answer = created
			answerMap[question.ID] = answer
		}

		var pts float64
		needsReview := false

		switch question.QuestionType {
		case "multiple_choice":
			correctIDs := make(map[uint64]bool)
			for _, c := range question.Choices {
				if c.IsCorrect {
					correctIDs[c.ID] = true
				}
			}
			studentIDs := ParseChoiceIDs(answer.AnswerText)
			studentSet := make(map[uint64]bool, len(studentIDs))
			for _, id := range studentIDs {
				studentSet[id] = true
			}
			if mapsEqual(correctIDs, studentSet) {
				pts = question.Weight
			}

		case "fill_in_blank":
			if strings.EqualFold(strings.TrimSpace(answer.AnswerText), strings.TrimSpace(question.CorrectAnswer)) {
				pts = question.Weight
			}

		case "short_answer", "long_answer":
			needsReview = true
		}

		score += pts
		ptsVal := pts
		updateFields := map[string]any{
			"points_awarded": &ptsVal,
			"needs_review":   needsReview,
		}
		if needsReview {
			updateFields["points_awarded"] = nil
		}
		if _, err := s.repo.UpdateAnswer(ctx, answer.ID, updateFields); err != nil {
			return nil, err
		}
	}

	now := time.Now()
	updatedSub, err := s.repo.UpdateSubmission(ctx, sub.ID, map[string]any{
		"status":       "submitted",
		"submitted_at": &now,
		"score":        score,
		"max_score":    maxScore,
	})
	if err != nil {
		return nil, err
	}

	// Create or update the grade record
	existingGrade, err := s.findGrade(ctx, quizID, callerUserID)
	if err != nil {
		return nil, err
	}
	if existingGrade == nil {
		newGrade := &grade.Grade{
			CourseID:  q.CourseID,
			StudentID: callerUserID,
			QuizID:    &quizID,
			Title:     q.Title,
			Score:     score,
			MaxScore:  maxScore,
		}
		created, err := s.createGrade(ctx, newGrade)
		if err != nil {
			return nil, err
		}
		gID := created.ID
		if _, err := s.repo.UpdateSubmission(ctx, sub.ID, map[string]any{"grade_id": gID}); err != nil {
			return nil, err
		}
		updatedSub.GradeID = &gID
	} else {
		if _, err := s.updateGrade(ctx, existingGrade.ID, map[string]any{"score": score, "max_score": maxScore}); err != nil {
			return nil, err
		}
	}

	finalAnswers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(updatedSub, finalAnswers), nil
}

// GetMySubmission returns the caller's submission for a quiz with answers and correct answers revealed.
func (s *Service) GetMySubmission(ctx context.Context, callerUserID uint64, quizID uint64) (*SubmissionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	if err := s.requireInHouseQuiz(q); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByQuizAndStudent(ctx, quizID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this quiz"}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(sub, answers), nil
}

// ─── Submission management (teacher/admin) ────────────────────────────────────

// ListSubmissions returns all submissions for a quiz.
func (s *Service) ListSubmissions(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) ([]*SubmissionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	course, err := s.findCourse(ctx, q.CourseID)
	if err != nil {
		return nil, err
	}
	if callerRole != auth.RoleAdmin && (course == nil || course.TeacherID != callerUserID) {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can view all submissions"}
	}
	subs, err := s.repo.FindSubmissionsByQuiz(ctx, quizID)
	if err != nil {
		return nil, err
	}
	out := make([]*SubmissionResponse, len(subs))
	for i, sub := range subs {
		answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
		if err != nil {
			return nil, err
		}
		out[i] = toSubmissionResponse(sub, answers)
	}
	return out, nil
}

// GetSubmission returns a single submission (teacher/admin or the owning student).
func (s *Service) GetSubmission(ctx context.Context, callerUserID uint64, callerRole string, quizID, submissionID uint64) (*SubmissionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	sub, err := s.repo.FindSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}
	if callerRole != auth.RoleAdmin && sub.StudentID != callerUserID {
		course, err := s.findCourse(ctx, q.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil || course.TeacherID != callerUserID {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Access denied"}
		}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(sub, answers), nil
}

// GradeAnswer manually grades a single answer (SA/LA) and recalculates the submission score.
func (s *Service) GradeAnswer(ctx context.Context, callerUserID uint64, callerRole string, quizID, submissionID, answerID uint64, p GradeAnswerParams) (*SubmissionResponse, error) {
	q, err := s.repo.FindByID(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUIZ_NOT_FOUND", Message: "Quiz not found"}
	}
	course, err := s.findCourse(ctx, q.CourseID)
	if err != nil {
		return nil, err
	}
	if callerRole != auth.RoleAdmin && (course == nil || course.TeacherID != callerUserID) {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can grade submissions"}
	}
	sub, err := s.repo.FindSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.QuizID != quizID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}

	// Find the answer and validate points
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	var targetAnswer *QuizAnswer
	for _, a := range answers {
		if a.ID == answerID {
			targetAnswer = a
			break
		}
	}
	if targetAnswer == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "ANSWER_NOT_FOUND", Message: "Answer not found"}
	}
	question, err := s.repo.FindQuestionByID(ctx, targetAnswer.QuestionID)
	if err != nil {
		return nil, err
	}
	if p.PointsAwarded < 0 || (question != nil && p.PointsAwarded > question.Weight) {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_POINTS", Message: "points_awarded cannot exceed the question weight"}
	}

	pts := p.PointsAwarded
	if _, err := s.repo.UpdateAnswer(ctx, answerID, map[string]any{
		"points_awarded": &pts,
		"needs_review":   false,
	}); err != nil {
		return nil, err
	}

	// Recalculate submission score
	updatedAnswers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	newScore := 0.0
	allGraded := true
	for _, a := range updatedAnswers {
		if a.PointsAwarded != nil {
			newScore += *a.PointsAwarded
		} else {
			allGraded = false
		}
	}

	newStatus := sub.Status
	if allGraded {
		newStatus = "graded"
	}

	updatedSub, err := s.repo.UpdateSubmission(ctx, sub.ID, map[string]any{
		"score":  newScore,
		"status": newStatus,
	})
	if err != nil {
		return nil, err
	}

	// Update grade record
	if sub.GradeID != nil {
		if _, err := s.updateGrade(ctx, *sub.GradeID, map[string]any{"score": newScore}); err != nil {
			return nil, err
		}
	}

	finalAnswers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(updatedSub, finalAnswers), nil
}

// ─── Utility ──────────────────────────────────────────────────────────────────

func mapsEqual(a, b map[uint64]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
