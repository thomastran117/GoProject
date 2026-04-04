package submission

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type submissionService interface {
	Submit(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64, p SubmitParams) (*SubmissionResponse, error)
	ListByAssignment(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64) ([]*SubmissionResponse, error)
	GetMine(ctx context.Context, callerUserID uint64, callerRole string, assignmentID uint64) (*SubmissionResponse, error)
	GradeSubmission(ctx context.Context, callerUserID uint64, callerRole string, assignmentID, submissionID uint64, p GradeParams) (*SubmissionResponse, error)
}

type submitRequest struct {
	BlobKey  string `json:"blob_key"  binding:"required,min=1,max=500"`
	FileName string `json:"file_name" binding:"required,min=1,max=300"`
}

type gradeRequest struct {
	Grade    uint   `json:"grade"`
	Feedback string `json:"feedback" binding:"omitempty,max=5000"`
}

// Handler holds the HTTP handlers for the submission resource.
type Handler struct {
	service submissionService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleSubmit handles POST /assignments/:id/submissions.
func (h *Handler) handleSubmit(c *gin.Context) {
	assignmentID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
	if err != nil {
		return
	}
	var req submitRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	result, err := h.service.Submit(c.Request.Context(), claims.UserID, claims.Role, assignmentID, SubmitParams{
		BlobKey:  req.BlobKey,
		FileName: req.FileName,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleList handles GET /assignments/:id/submissions.
func (h *Handler) handleList(c *gin.Context) {
	assignmentID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
	if err != nil {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	result, err := h.service.ListByAssignment(c.Request.Context(), claims.UserID, claims.Role, assignmentID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGetMine handles GET /assignments/:id/submissions/mine.
func (h *Handler) handleGetMine(c *gin.Context) {
	assignmentID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
	if err != nil {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	result, err := h.service.GetMine(c.Request.Context(), claims.UserID, claims.Role, assignmentID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleGrade handles PUT /assignments/:id/submissions/:subId.
func (h *Handler) handleGrade(c *gin.Context) {
	assignmentID, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
	if err != nil {
		return
	}
	submissionID, err := parseURLUint64(c, "subId", "INVALID_SUBMISSION_ID", "Invalid submission ID")
	if err != nil {
		return
	}
	var req gradeRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	result, err := h.service.GradeSubmission(c.Request.Context(), claims.UserID, claims.Role, assignmentID, submissionID, GradeParams{
		Grade:    req.Grade,
		Feedback: req.Feedback,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
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
