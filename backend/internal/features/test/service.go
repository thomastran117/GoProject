package test

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

type testRepository interface {
	Create(ctx context.Context, t *Test) (*Test, error)
	FindByID(ctx context.Context, id uint64) (*Test, error)
	FindByCourse(ctx context.Context, courseID uint64) ([]*Test, error)
	Update(ctx context.Context, id uint64, fields map[string]any) (*Test, error)
	Delete(ctx context.Context, id uint64) (bool, error)

	CreateQuestion(ctx context.Context, q *TestQuestion) (*TestQuestion, error)
	FindQuestionByID(ctx context.Context, id uint64) (*TestQuestion, error)
	FindQuestionsWithChoices(ctx context.Context, testID uint64) ([]*TestQuestion, error)
	UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*TestQuestion, error)
	DeleteQuestion(ctx context.Context, id uint64) (bool, error)
	DeleteChoicesByQuestion(ctx context.Context, questionID uint64) error
	CountQuestionsByTest(ctx context.Context, testID uint64) (int64, error)

	CreateChoice(ctx context.Context, c *TestChoice) (*TestChoice, error)
	FindChoiceByID(ctx context.Context, id uint64) (*TestChoice, error)
	FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*TestChoice, error)
	CountChoicesByQuestion(ctx context.Context, questionID uint64) (int64, error)
	UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*TestChoice, error)
	DeleteChoice(ctx context.Context, id uint64) (bool, error)

	CreateSubmission(ctx context.Context, s *TestSubmission) (*TestSubmission, error)
	FindSubmissionByID(ctx context.Context, id uint64) (*TestSubmission, error)
	FindSubmissionByTestAndStudent(ctx context.Context, testID, studentID uint64) (*TestSubmission, error)
	FindSubmissionsByTest(ctx context.Context, testID uint64) ([]*TestSubmission, error)
	CountActiveSubmissions(ctx context.Context, testID uint64) (int64, error)
	UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*TestSubmission, error)

	UpsertAnswer(ctx context.Context, a *TestAnswer) (*TestAnswer, error)
	FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*TestAnswer, error)
	UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*TestAnswer, error)
}

// Service implements business logic for tests.
type Service struct {
	repo        testRepository
	findCourse  func(ctx context.Context, id uint64) (*CourseInfo, error)
	isEnrolled  func(ctx context.Context, courseID, userID uint64) (bool, error)
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error)
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error)
	findGrade   func(ctx context.Context, testID, studentID uint64) (*grade.Grade, error)
}

func NewService(
	repo *Repository,
	findCourse func(ctx context.Context, id uint64) (*CourseInfo, error),
	isEnrolled func(ctx context.Context, courseID, userID uint64) (bool, error),
	createGrade func(ctx context.Context, g *grade.Grade) (*grade.Grade, error),
	updateGrade func(ctx context.Context, id uint64, fields map[string]any) (*grade.Grade, error),
	findGrade func(ctx context.Context, testID, studentID uint64) (*grade.Grade, error),
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

type TestResponse struct {
	ID          uint64     `json:"id"`
	CourseID    uint64     `json:"course_id"`
	AuthorID    uint64     `json:"author_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	TestType    string     `json:"test_type"`
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
	TestID      uint64           `json:"test_id"`
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

type StartTestResponse struct {
	Submission *SubmissionResponse        `json:"submission"`
	Questions  []*StudentQuestionResponse `json:"questions"`
}

// ─── Params ───────────────────────────────────────────────────────────────────

type CreateTestParams struct {
	Title       string
	Description string
	TestType    string
	ExternalURL string
	Status      string
	DueAt       *time.Time
}

type UpdateTestParams struct {
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

var validTestTypes = map[string]bool{"link": true, "in_house": true}
var validQuestionTypes = map[string]bool{
	"multiple_choice": true,
	"short_answer":    true,
	"fill_in_blank":   true,
	"long_answer":     true,
}
var validStatuses = map[string]bool{"draft": true, "published": true, "closed": true}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toTestResponse(t *Test) *TestResponse {
	return &TestResponse{
		ID:          t.ID,
		CourseID:    t.CourseID,
		AuthorID:    t.AuthorID,
		Title:       t.Title,
		Description: t.Description,
		TestType:    t.TestType,
		ExternalURL: t.ExternalURL,
		Status:      t.Status,
		DueAt:       t.DueAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func toQuestionResponse(q *TestQuestion) *QuestionResponse {
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

func toStudentQuestionResponse(q *TestQuestion) *StudentQuestionResponse {
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

func toAnswerResponse(a *TestAnswer) *AnswerResponse {
	return &AnswerResponse{
		ID:            a.ID,
		QuestionID:    a.QuestionID,
		AnswerText:    a.AnswerText,
		PointsAwarded: a.PointsAwarded,
		NeedsReview:   a.NeedsReview,
	}
}

func toSubmissionResponse(s *TestSubmission, answers []*TestAnswer) *SubmissionResponse {
	resp := &SubmissionResponse{
		ID:          s.ID,
		TestID:      s.TestID,
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

func sumWeights(questions []*TestQuestion) float64 {
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
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can manage tests"}
		}
	}
	return course, nil
}

func (s *Service) requireTestAuthorOrAdmin(ctx context.Context, testID, callerUserID uint64, callerRole string) (*Test, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if callerRole != auth.RoleAdmin && t.AuthorID != callerUserID {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the test author or an admin can perform this action"}
	}
	return t, nil
}

func (s *Service) requireInHouseTest(t *Test) error {
	if t.TestType == "link" {
		return &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "NOT_APPLICABLE", Message: "This operation is not applicable to link-type tests"}
	}
	return nil
}

// ─── Test CRUD ────────────────────────────────────────────────────────────────

func (s *Service) CreateTest(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateTestParams) (*TestResponse, error) {
	if _, err := s.requireCourseTeacherOrAdmin(ctx, courseID, callerUserID, callerRole); err != nil {
		return nil, err
	}
	if !validTestTypes[p.TestType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_TEST_TYPE", Message: "test_type must be 'link' or 'in_house'"}
	}
	if p.TestType == "link" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url is required for link-type tests"}
	}
	if p.TestType == "in_house" && strings.TrimSpace(p.ExternalURL) != "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_FORBIDDEN", Message: "external_url must not be set for in_house tests"}
	}
	status := p.Status
	if status == "" {
		status = "draft"
	}
	if !validStatuses[status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	t := &Test{
		CourseID:    courseID,
		AuthorID:    callerUserID,
		Title:       p.Title,
		Description: p.Description,
		TestType:    p.TestType,
		ExternalURL: p.ExternalURL,
		Status:      status,
		DueAt:       p.DueAt,
	}
	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	return toTestResponse(created), nil
}

func (s *Service) GetTest(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*TestResponse, error) {
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if callerRole != auth.RoleAdmin {
		course, err := s.findCourse(ctx, t.CourseID)
		if err != nil {
			return nil, err
		}
		if course == nil || course.TeacherID != callerUserID {
			enrolled, err := s.isEnrolled(ctx, t.CourseID, callerUserID)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its tests"}
			}
		}
	}
	resp := toTestResponse(t)
	questions, err := s.repo.FindQuestionsWithChoices(ctx, id)
	if err != nil {
		return nil, err
	}
	resp.TotalWeight = sumWeights(questions)
	return resp, nil
}

func (s *Service) ListByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*TestResponse, error) {
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
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled in this course to view its tests"}
		}
	}
	tests, err := s.repo.FindByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make([]*TestResponse, len(tests))
	for i, t := range tests {
		out[i] = toTestResponse(t)
	}
	return out, nil
}

func (s *Service) UpdateTest(ctx context.Context, callerUserID uint64, callerRole string, id uint64, p UpdateTestParams) (*TestResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if p.Status != "" && !validStatuses[p.Status] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATUS", Message: "status must be one of: draft, published, closed"}
	}
	if t.TestType == "link" && p.ExternalURL != "" && strings.TrimSpace(p.ExternalURL) == "" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "EXTERNAL_URL_REQUIRED", Message: "external_url cannot be cleared for a link-type test"}
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
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	return toTestResponse(updated), nil
}

func (s *Service) DeleteTest(ctx context.Context, callerUserID uint64, callerRole string, id uint64) error {
	if _, err := s.requireTestAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return err
	}
	active, err := s.repo.CountActiveSubmissions(ctx, id)
	if err != nil {
		return err
	}
	if active > 0 {
		return &middleware.APIError{Status: http.StatusConflict, Code: "TEST_HAS_SUBMISSIONS", Message: "Cannot delete a test with active submissions"}
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

func (s *Service) PublishTest(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*TestResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, id, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if t.TestType == "in_house" {
		count, err := s.repo.CountQuestionsByTest(ctx, id)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "TEST_HAS_NO_QUESTIONS", Message: "Cannot publish an in_house test with no questions"}
		}
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "published"})
	if err != nil {
		return nil, err
	}
	return toTestResponse(updated), nil
}

func (s *Service) CloseTest(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*TestResponse, error) {
	if _, err := s.requireTestAuthorOrAdmin(ctx, id, callerUserID, callerRole); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, id, map[string]any{"status": "closed"})
	if err != nil {
		return nil, err
	}
	return toTestResponse(updated), nil
}

// ─── Question management ──────────────────────────────────────────────────────

func (s *Service) ListQuestions(ctx context.Context, callerUserID uint64, callerRole string, testID uint64) ([]*QuestionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if callerRole != auth.RoleAdmin && t.AuthorID != callerUserID {
		enrolled, err := s.isEnrolled(ctx, t.CourseID, callerUserID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to view test questions"}
		}
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, testID)
	if err != nil {
		return nil, err
	}
	out := make([]*QuestionResponse, len(questions))
	for i, q := range questions {
		out[i] = toQuestionResponse(q)
	}
	return out, nil
}

func (s *Service) CreateQuestion(ctx context.Context, callerUserID uint64, callerRole string, testID uint64, p CreateQuestionParams) (*QuestionResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	if !validQuestionTypes[p.QuestionType] {
		return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: "INVALID_QUESTION_TYPE", Message: "question_type must be one of: multiple_choice, short_answer, fill_in_blank, long_answer"}
	}
	if p.Weight <= 0 {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "INVALID_WEIGHT", Message: "weight must be greater than 0"}
	}
	question := &TestQuestion{
		TestID:        testID,
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

func (s *Service) UpdateQuestion(ctx context.Context, callerUserID uint64, callerRole string, testID, questionID uint64, p UpdateQuestionParams) (*QuestionResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.TestID != testID {
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
	questions, err := s.repo.FindQuestionsWithChoices(ctx, testID)
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

func (s *Service) DeleteQuestion(ctx context.Context, callerUserID uint64, callerRole string, testID, questionID uint64) error {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.TestID != testID {
		return &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	if err := s.repo.DeleteChoicesByQuestion(ctx, questionID); err != nil {
		return err
	}
	_, err = s.repo.DeleteQuestion(ctx, questionID)
	return err
}

// ─── Choice management ────────────────────────────────────────────────────────

func (s *Service) CreateChoice(ctx context.Context, callerUserID uint64, callerRole string, testID, questionID uint64, p CreateChoiceParams) (*ChoiceResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.TestID != testID {
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
	choice := &TestChoice{QuestionID: questionID, SortOrder: p.SortOrder, Text: p.Text, IsCorrect: p.IsCorrect}
	created, err := s.repo.CreateChoice(ctx, choice)
	if err != nil {
		return nil, err
	}
	return &ChoiceResponse{ID: created.ID, SortOrder: created.SortOrder, Text: created.Text, IsCorrect: created.IsCorrect}, nil
}

func (s *Service) UpdateChoice(ctx context.Context, callerUserID uint64, callerRole string, testID, questionID, choiceID uint64, p UpdateChoiceParams) (*ChoiceResponse, error) {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return nil, err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.TestID != testID {
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

func (s *Service) DeleteChoice(ctx context.Context, callerUserID uint64, callerRole string, testID, questionID, choiceID uint64) error {
	t, err := s.requireTestAuthorOrAdmin(ctx, testID, callerUserID, callerRole)
	if err != nil {
		return err
	}
	if err := s.requireInHouseTest(t); err != nil {
		return err
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if question == nil || question.TestID != testID {
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

func (s *Service) StartTest(ctx context.Context, callerUserID uint64, callerRole string, testID uint64) (*StartTestResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	if t.Status != "published" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "TEST_NOT_PUBLISHED", Message: "This test is not published"}
	}
	if t.DueAt != nil && time.Now().After(*t.DueAt) {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "TEST_PAST_DUE", Message: "This test is past its due date"}
	}
	enrolled, err := s.isEnrolled(ctx, t.CourseID, callerUserID)
	if err != nil {
		return nil, err
	}
	if !enrolled {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "You must be enrolled to take this test"}
	}
	sub := &TestSubmission{TestID: testID, StudentID: callerUserID, StartedAt: time.Now()}
	created, err := s.repo.CreateSubmission(ctx, sub)
	if err != nil {
		return nil, err
	}
	if created.ID == 0 {
		return nil, &middleware.APIError{Status: http.StatusConflict, Code: "TEST_ALREADY_STARTED", Message: "You have already started this test"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, testID)
	if err != nil {
		return nil, err
	}
	studentQuestions := make([]*StudentQuestionResponse, len(questions))
	for i, q := range questions {
		studentQuestions[i] = toStudentQuestionResponse(q)
	}
	return &StartTestResponse{Submission: toSubmissionResponse(created, nil), Questions: studentQuestions}, nil
}

func (s *Service) SaveAnswer(ctx context.Context, callerUserID uint64, testID, questionID uint64, answerText string) (*AnswerResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByTestAndStudent(ctx, testID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this test"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "TEST_ALREADY_SUBMITTED", Message: "This test has already been submitted"}
	}
	question, err := s.repo.FindQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question == nil || question.TestID != testID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "QUESTION_NOT_FOUND", Message: "Question not found"}
	}
	answer, err := s.repo.UpsertAnswer(ctx, &TestAnswer{SubmissionID: sub.ID, QuestionID: questionID, AnswerText: answerText})
	if err != nil {
		return nil, err
	}
	return toAnswerResponse(answer), nil
}

func (s *Service) SubmitTest(ctx context.Context, callerUserID uint64, testID uint64) (*SubmissionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByTestAndStudent(ctx, testID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this test"}
	}
	if sub.Status != "in_progress" {
		return nil, &middleware.APIError{Status: http.StatusUnprocessableEntity, Code: "TEST_NOT_IN_PROGRESS", Message: "This submission is not in progress"}
	}
	questions, err := s.repo.FindQuestionsWithChoices(ctx, testID)
	if err != nil {
		return nil, err
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	answerMap := make(map[uint64]*TestAnswer, len(answers))
	for _, a := range answers {
		answerMap[a.QuestionID] = a
	}
	maxScore := sumWeights(questions)
	score := 0.0
	for _, question := range questions {
		answer, exists := answerMap[question.ID]
		if !exists {
			blank := &TestAnswer{SubmissionID: sub.ID, QuestionID: question.ID, AnswerText: ""}
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
		"status":       "submitted",
		"submitted_at": &now,
		"score":        score,
		"max_score":    maxScore,
	})
	if err != nil {
		return nil, err
	}
	existingGrade, err := s.findGrade(ctx, testID, callerUserID)
	if err != nil {
		return nil, err
	}
	if existingGrade == nil {
		newGrade := &grade.Grade{CourseID: t.CourseID, StudentID: callerUserID, TestID: &testID, Title: t.Title, Score: score, MaxScore: maxScore}
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

func (s *Service) GetMySubmission(ctx context.Context, callerUserID uint64, testID uint64) (*SubmissionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	if err := s.requireInHouseTest(t); err != nil {
		return nil, err
	}
	sub, err := s.repo.FindSubmissionByTestAndStudent(ctx, testID, callerUserID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "You have not started this test"}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	return toSubmissionResponse(sub, answers), nil
}

// ─── Submission management (teacher/admin) ────────────────────────────────────

func (s *Service) ListSubmissions(ctx context.Context, callerUserID uint64, callerRole string, testID uint64) ([]*SubmissionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	course, err := s.findCourse(ctx, t.CourseID)
	if err != nil {
		return nil, err
	}
	if callerRole != auth.RoleAdmin && (course == nil || course.TeacherID != callerUserID) {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Only the course teacher or an admin can view all submissions"}
	}
	subs, err := s.repo.FindSubmissionsByTest(ctx, testID)
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

func (s *Service) GetSubmission(ctx context.Context, callerUserID uint64, callerRole string, testID, submissionID uint64) (*SubmissionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	sub, err := s.repo.FindSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.TestID != testID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}
	if callerRole != auth.RoleAdmin && sub.StudentID != callerUserID {
		course, err := s.findCourse(ctx, t.CourseID)
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

func (s *Service) GradeAnswer(ctx context.Context, callerUserID uint64, callerRole string, testID, submissionID, answerID uint64, p GradeAnswerParams) (*SubmissionResponse, error) {
	t, err := s.repo.FindByID(ctx, testID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "Test not found"}
	}
	course, err := s.findCourse(ctx, t.CourseID)
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
	if sub == nil || sub.TestID != testID {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "SUBMISSION_NOT_FOUND", Message: "Submission not found"}
	}
	answers, err := s.repo.FindAnswersBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	var targetAnswer *TestAnswer
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
