package quiz

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/application/request"
	"backend/internal/features/token"

	"github.com/gin-gonic/gin"
)

// quizService is the interface the Handler depends on.
type quizService interface {
	CreateQuiz(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateQuizParams) (*QuizResponse, error)
	GetQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error)
	ListByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*QuizResponse, error)
	UpdateQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64, p UpdateQuizParams) (*QuizResponse, error)
	DeleteQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) error
	PublishQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error)
	CloseQuiz(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*QuizResponse, error)

	ListQuestions(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) ([]*QuestionResponse, error)
	CreateQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64, p CreateQuestionParams) (*QuestionResponse, error)
	UpdateQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64, p UpdateQuestionParams) (*QuestionResponse, error)
	DeleteQuestion(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64) error

	CreateChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID uint64, p CreateChoiceParams) (*ChoiceResponse, error)
	UpdateChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID, choiceID uint64, p UpdateChoiceParams) (*ChoiceResponse, error)
	DeleteChoice(ctx context.Context, callerUserID uint64, callerRole string, quizID, questionID, choiceID uint64) error

	StartQuiz(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) (*StartQuizResponse, error)
	SaveAnswer(ctx context.Context, callerUserID uint64, quizID, questionID uint64, answerText string) (*AnswerResponse, error)
	SubmitQuiz(ctx context.Context, callerUserID uint64, quizID uint64) (*SubmissionResponse, error)
	GetMySubmission(ctx context.Context, callerUserID uint64, quizID uint64) (*SubmissionResponse, error)

	ListSubmissions(ctx context.Context, callerUserID uint64, callerRole string, quizID uint64) ([]*SubmissionResponse, error)
	GetSubmission(ctx context.Context, callerUserID uint64, callerRole string, quizID, submissionID uint64) (*SubmissionResponse, error)
	GradeAnswer(ctx context.Context, callerUserID uint64, callerRole string, quizID, submissionID, answerID uint64, p GradeAnswerParams) (*SubmissionResponse, error)
}

// Handler holds the HTTP handlers for the quiz resource.
type Handler struct {
	service quizService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ─── Request bodies ───────────────────────────────────────────────────────────

type createQuizRequest struct {
	Title       string     `json:"title"        binding:"required,min=1,max=300"`
	Description string     `json:"description"`
	QuizType    string     `json:"quiz_type"    binding:"required"`
	ExternalURL string     `json:"external_url"`
	Status      string     `json:"status"`
	DueAt       *time.Time `json:"due_at"`
}

type updateQuizRequest struct {
	Title       string     `json:"title"        binding:"required,min=1,max=300"`
	Description string     `json:"description"`
	ExternalURL string     `json:"external_url"`
	Status      string     `json:"status"`
	DueAt       *time.Time `json:"due_at"`
}

type createQuestionRequest struct {
	SortOrder     int     `json:"sort_order"`
	QuestionType  string  `json:"question_type"  binding:"required"`
	Text          string  `json:"text"           binding:"required,min=1"`
	ImageBlobKey  string  `json:"image_blob_key"`
	Weight        float64 `json:"weight"         binding:"required"`
	CorrectAnswer string  `json:"correct_answer"`
}

type updateQuestionRequest struct {
	SortOrder     int     `json:"sort_order"`
	QuestionType  string  `json:"question_type"  binding:"required"`
	Text          string  `json:"text"           binding:"required,min=1"`
	ImageBlobKey  string  `json:"image_blob_key"`
	Weight        float64 `json:"weight"         binding:"required"`
	CorrectAnswer string  `json:"correct_answer"`
}

type createChoiceRequest struct {
	SortOrder int    `json:"sort_order"`
	Text      string `json:"text"      binding:"required,min=1,max=1000"`
	IsCorrect bool   `json:"is_correct"`
}

type updateChoiceRequest struct {
	SortOrder int    `json:"sort_order"`
	Text      string `json:"text"      binding:"required,min=1,max=1000"`
	IsCorrect bool   `json:"is_correct"`
}

type saveAnswerRequest struct {
	AnswerText string `json:"answer_text"`
}

type gradeAnswerRequest struct {
	PointsAwarded float64 `json:"points_awarded" binding:"min=0"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func getClaims(c *gin.Context) (*token.AccessClaims, bool) {
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
	}
	return claims, ok
}

func parseURLUint64(c *gin.Context, param, code, message string) (uint64, error) {
	id, err := strconv.ParseUint(c.Param(param), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": code, "message": message},
		})
		return 0, err
	}
	return id, nil
}

// ─── Quiz handlers ────────────────────────────────────────────────────────────

// handleCreate handles POST /courses/:courseId/quizzes
func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createQuizRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CreateQuiz(c.Request.Context(), claims.UserID, claims.Role, courseID, CreateQuizParams{
		Title:       req.Title,
		Description: req.Description,
		QuizType:    req.QuizType,
		ExternalURL: req.ExternalURL,
		Status:      req.Status,
		DueAt:       req.DueAt,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleListByCourse handles GET /courses/:courseId/quizzes
func (h *Handler) handleListByCourse(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListByCourse(c.Request.Context(), claims.UserID, claims.Role, courseID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGet handles GET /quizzes/:id
func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetQuiz(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleUpdate handles PUT /quizzes/:id
func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	var req updateQuizRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.UpdateQuiz(c.Request.Context(), claims.UserID, claims.Role, id, UpdateQuizParams{
		Title:       req.Title,
		Description: req.Description,
		ExternalURL: req.ExternalURL,
		Status:      req.Status,
		DueAt:       req.DueAt,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDelete handles DELETE /quizzes/:id
func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	if err := h.service.DeleteQuiz(c.Request.Context(), claims.UserID, claims.Role, id); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handlePublish handles POST /quizzes/:id/publish
func (h *Handler) handlePublish(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.PublishQuiz(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleClose handles POST /quizzes/:id/close
func (h *Handler) handleClose(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CloseQuiz(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ─── Question handlers ────────────────────────────────────────────────────────

// handleListQuestions handles GET /quizzes/:id/questions
func (h *Handler) handleListQuestions(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListQuestions(c.Request.Context(), claims.UserID, claims.Role, quizID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleCreateQuestion handles POST /quizzes/:id/questions
func (h *Handler) handleCreateQuestion(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	var req createQuestionRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CreateQuestion(c.Request.Context(), claims.UserID, claims.Role, quizID, CreateQuestionParams{
		SortOrder:     req.SortOrder,
		QuestionType:  req.QuestionType,
		Text:          req.Text,
		ImageBlobKey:  req.ImageBlobKey,
		Weight:        req.Weight,
		CorrectAnswer: req.CorrectAnswer,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleUpdateQuestion handles PUT /quizzes/:id/questions/:questionId
func (h *Handler) handleUpdateQuestion(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	var req updateQuestionRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.UpdateQuestion(c.Request.Context(), claims.UserID, claims.Role, quizID, questionID, UpdateQuestionParams{
		SortOrder:     req.SortOrder,
		QuestionType:  req.QuestionType,
		Text:          req.Text,
		ImageBlobKey:  req.ImageBlobKey,
		Weight:        req.Weight,
		CorrectAnswer: req.CorrectAnswer,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDeleteQuestion handles DELETE /quizzes/:id/questions/:questionId
func (h *Handler) handleDeleteQuestion(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	if err := h.service.DeleteQuestion(c.Request.Context(), claims.UserID, claims.Role, quizID, questionID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Choice handlers ──────────────────────────────────────────────────────────

// handleCreateChoice handles POST /quizzes/:id/questions/:questionId/choices
func (h *Handler) handleCreateChoice(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	var req createChoiceRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CreateChoice(c.Request.Context(), claims.UserID, claims.Role, quizID, questionID, CreateChoiceParams{
		SortOrder: req.SortOrder,
		Text:      req.Text,
		IsCorrect: req.IsCorrect,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleUpdateChoice handles PUT /quizzes/:id/questions/:questionId/choices/:choiceId
func (h *Handler) handleUpdateChoice(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	choiceID, err := parseURLUint64(c, "choiceId", "INVALID_CHOICE_ID", "Invalid choice ID")
	if err != nil {
		return
	}
	var req updateChoiceRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.UpdateChoice(c.Request.Context(), claims.UserID, claims.Role, quizID, questionID, choiceID, UpdateChoiceParams{
		SortOrder: req.SortOrder,
		Text:      req.Text,
		IsCorrect: req.IsCorrect,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDeleteChoice handles DELETE /quizzes/:id/questions/:questionId/choices/:choiceId
func (h *Handler) handleDeleteChoice(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	choiceID, err := parseURLUint64(c, "choiceId", "INVALID_CHOICE_ID", "Invalid choice ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	if err := h.service.DeleteChoice(c.Request.Context(), claims.UserID, claims.Role, quizID, questionID, choiceID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Student handlers ─────────────────────────────────────────────────────────

// handleStart handles POST /quizzes/:id/start
func (h *Handler) handleStart(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.StartQuiz(c.Request.Context(), claims.UserID, claims.Role, quizID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleSaveAnswer handles PUT /quizzes/:id/answers/:questionId
func (h *Handler) handleSaveAnswer(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	questionID, err := parseURLUint64(c, "questionId", "INVALID_QUESTION_ID", "Invalid question ID")
	if err != nil {
		return
	}
	var req saveAnswerRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.SaveAnswer(c.Request.Context(), claims.UserID, quizID, questionID, req.AnswerText)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleSubmit handles POST /quizzes/:id/submit
func (h *Handler) handleSubmit(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.SubmitQuiz(c.Request.Context(), claims.UserID, quizID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGetMySubmission handles GET /quizzes/:id/my-submission
func (h *Handler) handleGetMySubmission(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetMySubmission(c.Request.Context(), claims.UserID, quizID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ─── Submission handlers (teacher/admin) ──────────────────────────────────────

// handleListSubmissions handles GET /quizzes/:id/submissions
func (h *Handler) handleListSubmissions(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListSubmissions(c.Request.Context(), claims.UserID, claims.Role, quizID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGetSubmission handles GET /quizzes/:id/submissions/:submissionId
func (h *Handler) handleGetSubmission(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	submissionID, err := parseURLUint64(c, "submissionId", "INVALID_SUBMISSION_ID", "Invalid submission ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetSubmission(c.Request.Context(), claims.UserID, claims.Role, quizID, submissionID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGradeAnswer handles PUT /quizzes/:id/submissions/:submissionId/answers/:answerId
func (h *Handler) handleGradeAnswer(c *gin.Context) {
	quizID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid quiz ID")
	if err != nil {
		return
	}
	submissionID, err := parseURLUint64(c, "submissionId", "INVALID_SUBMISSION_ID", "Invalid submission ID")
	if err != nil {
		return
	}
	answerID, err := parseURLUint64(c, "answerId", "INVALID_ANSWER_ID", "Invalid answer ID")
	if err != nil {
		return
	}
	var req gradeAnswerRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GradeAnswer(c.Request.Context(), claims.UserID, claims.Role, quizID, submissionID, answerID, GradeAnswerParams{
		PointsAwarded: req.PointsAwarded,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
