package grade

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/application/middleware"
	"backend/internal/application/request"
	"backend/internal/features/token"

	"github.com/gin-gonic/gin"
)

type gradeService interface {
	CreateGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateGradeParams) (*GradeResponse, error)
	ListAll(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) ([]*StudentGradesResponse, error)
	GetMine(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64) (*MyGradesResponse, error)
	UpdateGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID, gradeID uint64, p UpdateGradeParams) (*GradeResponse, error)
	DeleteGrade(ctx context.Context, callerUserID uint64, callerRole string, courseID, gradeID uint64) error
}

type createGradeRequest struct {
	StudentID    uint64  `json:"student_id"    binding:"required"`
	AssignmentID *uint64 `json:"assignment_id"`
	QuizID       *uint64 `json:"quiz_id"`
	TestID       *uint64 `json:"test_id"`
	ExamID       *uint64 `json:"exam_id"`
	Title        string  `json:"title"         binding:"required,min=1,max=300"`
	Score        float64 `json:"score"         binding:"min=0"`
	MaxScore     float64 `json:"max_score"     binding:"omitempty,min=0"`
}

type updateGradeRequest struct {
	Title    *string  `json:"title"     binding:"omitempty,min=1,max=300"`
	Score    *float64 `json:"score"     binding:"omitempty,min=0"`
	MaxScore *float64 `json:"max_score" binding:"omitempty,min=0"`

	// FK fields are present only to detect and reject mutation attempts.
	// They must not appear in the service params.
	AssignmentID *uint64 `json:"assignment_id"`
	QuizID       *uint64 `json:"quiz_id"`
	TestID       *uint64 `json:"test_id"`
	ExamID       *uint64 `json:"exam_id"`
}

// Handler holds the HTTP handlers for the grade resource.
type Handler struct {
	service gradeService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// getClaims extracts JWT claims from the context, writing a 401 response and
// returning false when the claims are absent.
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

// handleCreate handles POST /courses/:id/grades.
func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createGradeRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.CreateGrade(c.Request.Context(), claims.UserID, claims.Role, courseID, CreateGradeParams{
		StudentID:    req.StudentID,
		AssignmentID: req.AssignmentID,
		QuizID:       req.QuizID,
		TestID:       req.TestID,
		ExamID:       req.ExamID,
		Title:        req.Title,
		Score:        req.Score,
		MaxScore:     req.MaxScore,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleListAll handles GET /courses/:id/grades.
func (h *Handler) handleListAll(c *gin.Context) {
	courseID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid course ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.ListAll(c.Request.Context(), claims.UserID, claims.Role, courseID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGetMine handles GET /courses/:id/grades/mine.
func (h *Handler) handleGetMine(c *gin.Context) {
	courseID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid course ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.GetMine(c.Request.Context(), claims.UserID, claims.Role, courseID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleUpdate handles PUT /courses/:id/grades/:gradeId.
func (h *Handler) handleUpdate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid course ID")
	if err != nil {
		return
	}
	gradeID, err := parseURLUint64(c, "gradeId", "INVALID_GRADE_ID", "Invalid grade ID")
	if err != nil {
		return
	}
	var req updateGradeRequest
	if !request.BindJSON(c, &req) {
		return
	}
	if req.AssignmentID != nil || req.QuizID != nil || req.TestID != nil || req.ExamID != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error":   gin.H{"code": "IMMUTABLE_FIELD", "message": "FK reference fields cannot be changed after creation"},
		})
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	result, err := h.service.UpdateGrade(c.Request.Context(), claims.UserID, claims.Role, courseID, gradeID, UpdateGradeParams{
		Title:    req.Title,
		Score:    req.Score,
		MaxScore: req.MaxScore,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDelete handles DELETE /courses/:id/grades/:gradeId.
func (h *Handler) handleDelete(c *gin.Context) {
	courseID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid course ID")
	if err != nil {
		return
	}
	gradeID, err := parseURLUint64(c, "gradeId", "INVALID_GRADE_ID", "Invalid grade ID")
	if err != nil {
		return
	}
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	if err := h.service.DeleteGrade(c.Request.Context(), claims.UserID, claims.Role, courseID, gradeID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// parseURLUint64 parses a named URL parameter as a uint64, writing a 400
// response and returning a non-nil error when the value is invalid.
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
