package exam

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

type examRepository interface {
	Create(ctx context.Context, e *Exam) (*Exam, error)
	FindByID(ctx context.Context, id uint64) (*Exam, error)
	FindByCourse(ctx context.Context, courseID uint64) ([]*Exam, error)
	Update(ctx context.Context, id uint64, fields map[string]any) (*Exam, error)
	Delete(ctx context.Context, id uint64) (bool, error)

	CreateQuestion(ctx context.Context, q *ExamQuestion) (*ExamQuestion, error)
	FindQuestionByID(ctx context.Context, id uint64) (*ExamQuestion, error)
	FindQuestionsWithChoices(ctx context.Context, examID uint64) ([]*ExamQuestion, error)
	UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*ExamQuestion, error)
	DeleteQuestion(ctx context.Context, id uint64) (bool, error)
	DeleteChoicesByQuestion(ctx context.Context, questionID uint64) error
	CountQuestionsByExam(ctx context.Context, examID uint64) (int64, error)

	CreateChoice(ctx context.Context, c *ExamChoice) (*ExamChoice, error)
	FindChoiceByID(ctx context.Context, id uint64) (*ExamChoice, error)
	FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*ExamChoice, error)
	CountChoicesByQuestion(ctx context.Context, questionID uint64) (int64, error)
	UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*ExamChoice, error)
	DeleteChoice(ctx context.Context, id uint64) (bool, error)

	CreateSubmission(ctx context.Context, s *ExamSubmission) (*ExamSubmission, error)
	FindSubmissionByID(ctx context.Context, id uint64) (*ExamSubmission, error)
	FindSubmissionByExamAndStudent(ctx context.Context, examID, studentID uint64) (*ExamSubmission, error)
	FindSubmissionsByExam(ctx context.Context, examID uint64) ([]*ExamSubmission, error)
	CountActiveSubmissions(ctx context.Context, examID uint64) (int64, error)
	UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*ExamSubmission, error)

	UpsertAnswer(ctx context.Context, a *ExamAnswer) (*ExamAnswer, error)
	FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*ExamAnswer, error)
	UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*ExamAnswer, error)
}

// Service implements business logic for exams.
type Service struct {
	repo        examRepository
	findCourse  func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled  func(ctx context.Context, courseID, userID uint64) (bool, error)
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error)
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error)
	findGrade   func(ctx context.Context, examID, studentID uint64) (*grade.Grade, error)
}

func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error),
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error),
	findGrade func(ctx context.Context, examID, studentID uint64) (*grade.Grade, error),
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

type ExamResponse struct {
	ID          uint64     `json:"id"`
	CourseID    uint64     `json:"course_id"`
	AuthorID    uint64     `json:"author_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ExamType    string     `json:"exam_type"`
	ExternalURL string     `json:"external_url,omitempty"`
	Status      string     `json:"status"`
	DueAt       *time.Time `json:"due_at"`
	TotalWeight float64    `json:"total_weight"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
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
	ExamID      uint64           `json:"exam_id"`
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

type StartExamResponse struct {
	Submission *SubmissionResponse        `json:"submission"`
	Questions  []*StudentQuestionResponse `json:"questions"`
}

// ─── Params ───────────────────────────────────────────────────────────────────

type CreateExamParams struct {
	Title       string
	Description string
	ExamType    string
	ExternalURL string
	Status      string
	DueAt       *time.Time
}

type UpdateExamParams struct {
	Title       string
	Description string
	ExternalURL string
	Status      string
	DueAt       *time.Time
}

type CreateQuestionParams struct {
	SortOrder     int
	QuestionType  string
	Text          string
	ImageBlobKey  string
	Weight        float64
	CorrectAnswer string
}

type UpdateQuestionParams struct {
	SortOrder     int
	QuestionType  string
	Text          string
	ImageBlobKey  string
	Weight        float64
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

var validExamTypes = map[string]bool{"link": true, "in_house": true}
var validQuestionTypes = map[string]bool{
	"multiple_choice": true,
	"short_answer":    true,
	"fill_in_blank":   true,
	"long_answer":     true,
}
var validStatuses = map[string]bool{"draft": true, "published": true, "closed": true}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toExamResponse(e *Exam) *ExamResponse {
	return &ExamResponse{
		ID:          e.ID,
		CourseID:    e.CourseID,
		AuthorID:    e.AuthorID,
		Title:       e.Title,
		Description: e.Description,
		ExamType:    e.ExamType,
		ExternalURL: e.ExternalURL,
		Status:      e.Status,
		DueAt:       e.DueAt,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func toQuestionResponse(q *ExamQuestion) *QuestionResponse {
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
			ID: c.ID, SortOrder: c.SortOrder, Text: c.Text, IsCorrect: c.IsCorrect,
		})
	}
	return resp
}

func toStudentQuestionResponse(q *ExamQuestion) *StudentQuestionResponse {
	resp := &StudentQuestionResponse{
		ID: q.ID, SortOrder: q.SortOrder, QuestionType: q.QuestionType,
		Text: q.Text, ImageBlobKey: q.ImageBlobKey, Weight: q.Weight,
	}
	if q.QuestionType == "multiple_choice" {
		for _, c := range q.Choices {
			resp.Choices = append(resp.Choices, &StudentChoiceResponse{ID: c.ID, SortOrder: c.SortOrder, Text: c.Text})
		}
	}
	return resp
}

func toAnswerResponse(a *ExamAnswer) *AnswerResponse {
	return &AnswerResponse{
		ID: a.ID, QuestionID: a.QuestionID, AnswerText: a.AnswerText,
		PointsAwarded: a.PointsAwarded, NeedsReview: a.NeedsReview,
	}
}

func toSubmissionResponse(s *ExamSubmission, answers []*ExamAnswer) *SubmissionResponse {
	resp := &SubmissionResponse{
		ID: s.ID, ExamID: s.ExamID, StudentID: s.StudentID, Status: s.Status,
		StartedAt: s.StartedAt, SubmittedAt: s.SubmittedAt,
		Score: s.Score, MaxScore: s.MaxScore, GradeID: s.GradeID,
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

func sumWeights(questions []*ExamQuestion) float64 {
	total := 0.0
	for _, q := range questions {
		total += q.Weight
	}
	return total
}

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
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can manage exams"}
		}
	}
	return course, nil
}

func (s *Service) requireExamAuthorOrAdmin(ctx context.Context, examID, callerUserID uint64, callerRole string) (*Exam, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if callerRole != auth.RoleAdmin && e.AuthorID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the exam author or an admin can perform this action"}
	}
	return e, nil
}

func (s *Service) requireInHouseExam(e *Exam) error {
	if e.ExamType == "link" {
		return &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "NOT_APPLICABLE", Message: "This operation is not applicable to link-type exams"}
	}
	return nil
}

// ─── Exam CRUD ────────────────────────────────────────────────────────────────

func (s *Service) CreateExam(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateExamParams) (*ExamResponse, error) {
	if _, err := s.requireCourseTeacherOrAdmin(ctx, courseID, callerUserID, callerRole); err != nil {
		return nil, err
	}
	if !validExamTypes[p.ExamType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_EXAM_TYPE", Message: "exam_type must be 'link' or 'in_house'"}
	}
	if p.ExamType == "link" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url is required for link-type exams"}
	}
	if p.ExamType == "in_house" && strings.TrimSpace(p.ExternalURL) != "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_FORBIDDEN", Message: "external_url must not be set for in_house exams"}
	}
	status := p.Status
	if status == "" {
		status = "draft"
	}
	if !validStatuses[status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	e := &Exam{
		CourseID: courseID, AuthorID: callerUserID, Title: p.Title,
		Description: p.Description, ExamType: p.ExamType, ExternalURL: p.ExternalURL,
		Status: status, DueAt: p.DueAt,
	}
	created, err := s.repo.Create(ctx, e)
	if err != nil {
		return nil, err
	}
	return toExamResponse(created), nil
}

func (s *Service) GetExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error) {
	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if callerRole != auth.RoleAdmin {
		course, err := s.findCourse(ctx, e.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil || course.TeacherID != callerUserID {
			enrolled, err := s.isEnrolled(ctx, e.CourseID, callerUserID)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its exams"}
			}
		}
	}
	resp := toExamResponse(e)
	questions, err := s.repo.FindQuestionsWithChoices(ctx, id)
	if err != nil {
		return nil, err
	}
	resp.TotalWeight = sumWeights(questions)
	return resp, nil
}

func (s *Service) ListByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*ExamResponse, error) {
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
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its exams"}
		}
	}
	exams, err := s.repo.FindByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make([]*ExamResponse, len(exams))
	for i, e := range exams {
		out[i] = toExamResponse(e)
	}
	return out, nil
}

func (s *Service) UpdateExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64, p UpdateExamParams) (*ExamResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if p.Status != "" && !validStatuses[p.Status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	if e.ExamType == "link" && p.ExternalURL != "" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url cannot be cleared for a link-type exam"}
	}
	fields := map[string]any{
		"title": p.Title, "description": p.Description,
		"external_url": p.ExternalURL, "due_at": p.DueAt,
	}
	if p.Status != "" {
		fields["status"] = p.Status
	}
	updated, err := s.repo.Update(ctx, id, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	return toExamResponse(updated), nil
}

func (s *Service) DeleteExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) error {
	if _, err := s.requireExamAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return err
	}
	active, err := s.repo.CountActiveSubmissions(ctx, id)
	if err != nil {
		return err
	}
	if active > 0 {
		return &middleware.APIError{Status: http.StatusConflict, Code: "EXAM_HAS_SUBMISSIONS", Message: "Cannot delete an exam with active submissions"}
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

func (s *Service) PublishExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if e.ExamType == "in_house" {
		count, err := s.repo.CountQuestionsByExam(ctx, id)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXAM_HAS_NO_QUESTIONS", Message: "Cannot publish an in_house exam with no questions"}
		}
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "published"})
	if err != nil {
		return nil, err
	}
	return toExamResponse(updated), nil
}

func (s *Service) CloseExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error) {
	if _, err := s.requireExamAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "closed"})
	if err != nil {
		return nil, err
	}
	return toExamResponse(updated), nil
}

// ─── Question management ──────────────────────────────────────────────────────

func (s *Service) ListQuestions(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) ([]*QuestionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if callerRole != auth.RoleAdmin && e.AuthorID != callerUserID {
		enrolled, err := s.isEnrolled(ctx, e.CourseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to view exam questions"}
		}
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, examID)
	if err != nil {
		return nil, err
	}
	out := make([]*QuestionResponse, len(questions))
	for i, q := range questions {
		out[i] = toQuestionResponse(q)
	}
	return out, nil
}

func (s *Service) CreateQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID uint64, p CreateQuestionParams) (*QuestionResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	if !validQuestionTypes[p.QuestionType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUESTION_TYPE", Message: "question_type must be one of: multiple_choice, short_answer, fill_in_blank, long_answer"}
	}
	if p.Weight <= 0 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_WEIGHT", Message: "weight must be greater than 0"}
	}
	question := &ExamQuestion{
		ExamID: examID, SortOrder: p.SortOrder, QuestionType: p.QuestionType,
		Text: p.Text, ImageBlobKey: p.ImageBlobKey, Weight: p.Weight, CorrectAnswer: p.CorrectAnswer,
	}
	created, err := s.repo.CreateQuestion(ctx, question)
	if err != nil {
		return nil, err
	}
	return toQuestionResponse(created), nil
}

func (s *Service) UpdateQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64, p UpdateQuestionParams) (*QuestionResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.ExamID != examID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if p.QuestionType != "" && !validQuestionTypes[p.QuestionType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUESTION_TYPE", Message: "question_type must be one of: multiple_choice, short_answer, fill_in_blank, long_answer"}
	}
	if p.Weight <= 0 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_WEIGHT", Message: "weight must be greater than 0"}
	}
	fields := map[string]any{
		"sort_order": p.SortOrder, "question_type": p.QuestionType, "text": p.Text,
		"image_blob_key": p.ImageBlobKey, "weight": p.Weight, "correct_answer": p.CorrectAnswer,
	}
	updated, err := s.repo.UpdateQuestion(ctx, questionID, fields)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, examID)
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

func (s *Service) DeleteQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64) error {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.ExamID != examID {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if err := s.repo.DeleteChoicesByQuestion(ctx, questionID); err != nil {
		return err
	}
	_, err = s.repo.DeleteQuestion(ctx, questionID)
	return err
}

// ─── Choice management ────────────────────────────────────────────────────────

func (s *Service) CreateChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64, p CreateChoiceParams) (*ChoiceResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.ExamID != examID {
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
	choice := &ExamChoice{QuestionID: questionID, SortOrder: p.SortOrder, Text: p.Text, IsCorrect: p.IsCorrect}
	created, err := s.repo.CreateChoice(ctx, choice)
	if err != nil {
		return nil, err
	}
	return &ChoiceResponse{ID: created.ID, SortOrder: created.SortOrder, Text: created.Text, IsCorrect: created.IsCorrect}, nil
}

func (s *Service) UpdateChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID, choiceID uint64, p UpdateChoiceParams) (*ChoiceResponse, error) {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.ExamID != examID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	choice, err := s.repo.FindChoiceByID(ctx, choiceID)
	if err != nil {
		return nil, err
	}
	if choice == nil || choice.QuestionID != questionID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "CHOICE_NOT_FOUND", Message: "Choice not found"}
	}
	updated, err := s.repo.UpdateChoice(ctx, choiceID, map[string]any{"sort_order": p.SortOrder, "text": p.Text, "is_correct": p.IsCorrect})
	if err != nil {
		return nil, err
	}
	return &ChoiceResponse{ID: updated.ID, SortOrder: updated.SortOrder, Text: updated.Text, IsCorrect: updated.IsCorrect}, nil
}

func (s *Service) DeleteChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID, choiceID uint64) error {
	e, err := s.requireExamAuthorOrAdmin(ctx, examID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseExam(e); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.ExamID != examID {
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

func (s *Service) StartExam(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) (*StartExamResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	if e.Status != "published" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXAM_NOT_PUBLISHED", Message: "This exam is not published"}
	}
	if e.DueAt != nil && time.Now().After(*e.DueAt) {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXAM_PAST_DUE", Message: "This exam is past its due date"}
	}
	enrolled, err := s.isEnrolled(ctx, e.CourseID, callerUserID)
	if err != nil {
		return nil, err
	}
	if !enrolled {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to take this exam"}
	}
	sub := &ExamSubmission{ExamID: examID, StudentID: callerUserID, StartedAt: time.Now()}
	created, err := s.repo.CreateSubmission(ctx, sub)
	if err != nil {
		return nil, err
	}
	if created.ID == 0 {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "EXAM_ALREADY_STARTED", Message: "You have already started this exam"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, examID)
	if err != nil {
		return nil, err
	}
	studentQuestions := make([]*StudentQuestionResponse, len(questions))
	for i, q := range questions {
		studentQuestions[i] = toStudentQuestionResponse(q)
	}
	return &StartExamResponse{Submission: toSubmissionResponse(created, nil), Questions: studentQuestions}, nil
}

func (s *Service) SaveAnswer(ctx context.Context, callerUserID uint64, examID, questionID uint64, answerText string) (*AnswerResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByExamAndStudent(ctx, examID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this exam"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXAM_ALREADY_SUBMITTED", Message: "This exam has already been submitted"}
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.ExamID != examID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	answer, err := s.repo.UpsertAnswer(ctx, &ExamAnswer{SubmissionID: sub.ID, QuestionID: questionID, AnswerText: answerText})
	if err != nil {
		return nil, err
	}
	return toAnswerResponse(answer), nil
}

func (s *Service) SubmitExam(ctx context.Context, callerUserID uint64, examID uint64) (*SubmissionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByExamAndStudent(ctx, examID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this exam"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXAM_NOT_IN_PROGRESS", Message: "This submission is not in progress"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, examID)
	if err != nil {
		return nil, err
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	answerMap := make(map[uint64]*ExamAnswer, len(answers))
	for _, a := range answers {
		answerMap[a.QuestionID] = a
	}
	maxScore := sumWeights(questions)
	score := 0.0
	for _, question := range questions {
		answer, exists := answerMap[question.ID]
		if !exists {
			blank := &ExamAnswer{SubmissionID: sub.ID, QuestionID: question.ID, AnswerText: ""}
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
		updateFields := map[string]any{"points_awarded": &ptsVal, "needs_review": needsReview}
		if needsReview {
			updateFields["points_awarded"] = nil
		}
		if _, err := s.repo.UpdateAnswer(ctx, answer.ID, updateFields); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	updatedSub, err := s.repo.UpdateSubmission(ctx, sub.ID, map[string]any{
		"status": "submitted", "submitted_at": &now, "score": score, "max_score": maxScore,
	})
	if err != nil {
		return nil, err
	}
	existingGrade, err := s.findGrade(ctx, examID, callerUserID)
	if err != nil {
		return nil, err
	}
	if existingGrade == nil {
		newGrade := &grade.Grade{CourseID: e.CourseID, StudentID: callerUserID, ExamID: &examID, Title: e.Title, Score: score, MaxScore: maxScore}
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

func (s *Service) GetMySubmission(ctx context.Context, callerUserID uint64, examID uint64) (*SubmissionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	if err := s.requireInHouseExam(e); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByExamAndStudent(ctx, examID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this exam"}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(sub, answers), nil
}

// ─── Submission management (teacher/admin) ────────────────────────────────────

func (s *Service) ListSubmissions(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) ([]*SubmissionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	course, err := s.findCourse(ctx, e.CourseID)
	if err != nil {
		return nil, err
	}
	if callerRole != auth.RoleAdmin && (course == nil || course.TeacherID != callerUserID) {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can view all submissions"}
	}
	subs, err := s.repo.FindSubmissionsByExam(ctx, examID)
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

func (s *Service) GetSubmission(ctx context.Context, callerUserID uint64, callerRole string, examID, submissionID uint64) (*SubmissionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	sub, err := s.repo.FindSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.ExamID != examID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}
	if callerRole != auth.RoleAdmin && sub.StudentID != callerUserID {
		course, err := s.findCourse(ctx, e.CourseID)
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

func (s *Service) GradeAnswer(ctx context.Context, callerUserID uint64, callerRole string, examID, submissionID, answerID uint64, p GradeAnswerParams) (*SubmissionResponse, error) {
	e, err := s.repo.FindByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "EXAM_NOT_FOUND", Message: "Exam not found"}
	}
	course, err := s.findCourse(ctx, e.CourseID)
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
	if sub == nil || sub.ExamID != examID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	var targetAnswer *ExamAnswer
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
	if _, err := s.repo.UpdateAnswer(ctx, answerID, map[string]any{"points_awarded": &pts, "needs_review": false}); err != nil {
		return nil, err
	}
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
	updatedSub, err := s.repo.UpdateSubmission(ctx, sub.ID, map[string]any{"score": newScore, "status": newStatus})
	if err != nil {
		return nil, err
	}
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
