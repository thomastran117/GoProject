package exam

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

type examService interface {
	CreateExam(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateExamParams) (*ExamResponse, error)
	GetExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error)
	ListByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*ExamResponse, error)
	UpdateExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64, p UpdateExamParams) (*ExamResponse, error)
	DeleteExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) error
	PublishExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error)
	CloseExam(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*ExamResponse, error)

	ListQuestions(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) ([]*QuestionResponse, error)
	CreateQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID uint64, p CreateQuestionParams) (*QuestionResponse, error)
	UpdateQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64, p UpdateQuestionParams) (*QuestionResponse, error)
	DeleteQuestion(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64) error

	CreateChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID uint64, p CreateChoiceParams) (*ChoiceResponse, error)
	UpdateChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID, choiceID uint64, p UpdateChoiceParams) (*ChoiceResponse, error)
	DeleteChoice(ctx context.Context, callerUserID uint64, callerRole string, examID, questionID, choiceID uint64) error

	StartExam(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) (*StartExamResponse, error)
	SaveAnswer(ctx context.Context, callerUserID uint64, examID, questionID uint64, answerText string) (*AnswerResponse, error)
	SubmitExam(ctx context.Context, callerUserID uint64, examID uint64) (*SubmissionResponse, error)
	GetMySubmission(ctx context.Context, callerUserID uint64, examID uint64) (*SubmissionResponse, error)

	ListSubmissions(ctx context.Context, callerUserID uint64, callerRole string, examID uint64) ([]*SubmissionResponse, error)
	GetSubmission(ctx context.Context, callerUserID uint64, callerRole string, examID, submissionID uint64) (*SubmissionResponse, error)
	GradeAnswer(ctx context.Context, callerUserID uint64, callerRole string, examID, submissionID, answerID uint64, p GradeAnswerParams) (*SubmissionResponse, error)
}

// Handler holds the HTTP handlers for the exam resource.
type Handler struct {
	service examService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createExamRequest struct {
	Title       string     `json:"title"        binding:"required,min=1,max=300"`
	Description string     `json:"description"`
	ExamType    string     `json:"exam_type"    binding:"required"`
	ExternalURL string     `json:"external_url"`
	Status      string     `json:"status"`
	DueAt       *time.Time `json:"due_at"`
}

type updateExamRequest struct {
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

func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createExamRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CreateExam(c.Request.Context(), claims.UserID, claims.Role, courseID, CreateExamParams{
		Title: req.Title, Description: req.Description, ExamType: req.ExamType,
		ExternalURL: req.ExternalURL, Status: req.Status, DueAt: req.DueAt,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

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

func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetExam(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	var req updateExamRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.UpdateExam(c.Request.Context(), claims.UserID, claims.Role, id, UpdateExamParams{
		Title: req.Title, Description: req.Description, ExternalURL: req.ExternalURL, Status: req.Status, DueAt: req.DueAt,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	if err := h.service.DeleteExam(c.Request.Context(), claims.UserID, claims.Role, id); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) handlePublish(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.PublishExam(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleClose(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CloseExam(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleListQuestions(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListQuestions(c.Request.Context(), claims.UserID, claims.Role, examID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleCreateQuestion(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.CreateQuestion(c.Request.Context(), claims.UserID, claims.Role, examID, CreateQuestionParams{
		SortOrder: req.SortOrder, QuestionType: req.QuestionType, Text: req.Text,
		ImageBlobKey: req.ImageBlobKey, Weight: req.Weight, CorrectAnswer: req.CorrectAnswer,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

func (h *Handler) handleUpdateQuestion(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.UpdateQuestion(c.Request.Context(), claims.UserID, claims.Role, examID, questionID, UpdateQuestionParams{
		SortOrder: req.SortOrder, QuestionType: req.QuestionType, Text: req.Text,
		ImageBlobKey: req.ImageBlobKey, Weight: req.Weight, CorrectAnswer: req.CorrectAnswer,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleDeleteQuestion(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	if err := h.service.DeleteQuestion(c.Request.Context(), claims.UserID, claims.Role, examID, questionID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) handleCreateChoice(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.CreateChoice(c.Request.Context(), claims.UserID, claims.Role, examID, questionID, CreateChoiceParams{
		SortOrder: req.SortOrder, Text: req.Text, IsCorrect: req.IsCorrect,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

func (h *Handler) handleUpdateChoice(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.UpdateChoice(c.Request.Context(), claims.UserID, claims.Role, examID, questionID, choiceID, UpdateChoiceParams{
		SortOrder: req.SortOrder, Text: req.Text, IsCorrect: req.IsCorrect,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleDeleteChoice(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	if err := h.service.DeleteChoice(c.Request.Context(), claims.UserID, claims.Role, examID, questionID, choiceID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) handleStart(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.StartExam(c.Request.Context(), claims.UserID, claims.Role, examID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

func (h *Handler) handleSaveAnswer(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.SaveAnswer(c.Request.Context(), claims.UserID, examID, questionID, req.AnswerText)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleSubmit(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.SubmitExam(c.Request.Context(), claims.UserID, examID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleGetMySubmission(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetMySubmission(c.Request.Context(), claims.UserID, examID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleListSubmissions(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListSubmissions(c.Request.Context(), claims.UserID, claims.Role, examID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleGetSubmission(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.GetSubmission(c.Request.Context(), claims.UserID, claims.Role, examID, submissionID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) handleGradeAnswer(c *gin.Context) {
	examID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid exam ID")
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
	result, err := h.service.GradeAnswer(c.Request.Context(), claims.UserID, claims.Role, examID, submissionID, answerID, GradeAnswerParams{PointsAwarded: req.PointsAwarded})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
