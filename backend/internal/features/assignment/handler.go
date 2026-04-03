package assignment

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type assignmentService interface {
	Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*AssignmentResponse, error)
	GetByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, page, pageSize int) (*PagedResult, error)
	GetByID(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*AssignmentResponse, error)
	Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*AssignmentResponse, error)
	Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error
	Search(ctx context.Context, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error)
}

type createAssignmentRequest struct {
	Title       string     `json:"title"       binding:"required,min=1,max=300"`
	Description string     `json:"description" binding:"required,min=1"`
	DueAt       *time.Time `json:"due_at"`
	Points      uint       `json:"points"`
	Status      string     `json:"status"      binding:"omitempty,max=20"`
}

type updateAssignmentRequest struct {
	Title       string     `json:"title"       binding:"required,min=1,max=300"`
	Description string     `json:"description" binding:"required,min=1"`
	DueAt       *time.Time `json:"due_at"`
	Points      uint       `json:"points"`
	Status      string     `json:"status"      binding:"omitempty,max=20"`
}

// Handler holds the HTTP handlers for the assignment resource.
type Handler struct {
	service assignmentService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleCreate handles POST /courses/:courseId/assignments.
func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createAssignmentRequest
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
	params := CreateParams{
		Title:       req.Title,
		Description: req.Description,
		DueAt:       req.DueAt,
		Points:      req.Points,
		Status:      req.Status,
	}
	result, err := h.service.Create(c.Request.Context(), claims.UserID, claims.Role, courseID, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleGetByCourse handles GET /courses/:courseId/assignments.
func (h *Handler) handleGetByCourse(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
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
	page, pageSize := parsePagination(c)
	result, err := h.service.GetByCourse(c.Request.Context(), claims.UserID, claims.Role, courseID, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result.Data, "pagination": result.Pagination})
}

// handleGet handles GET /assignments/:id.
func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
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
	result, err := h.service.GetByID(c.Request.Context(), claims.UserID, claims.Role, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleUpdate handles PUT /assignments/:id.
func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
	if err != nil {
		return
	}
	var req updateAssignmentRequest
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
	params := UpdateParams{
		Title:       req.Title,
		Description: req.Description,
		DueAt:       req.DueAt,
		Points:      req.Points,
		Status:      req.Status,
	}
	result, err := h.service.Update(c.Request.Context(), id, claims.UserID, claims.Role, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDelete handles DELETE /assignments/:id.
func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid assignment ID")
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
	if err := h.service.Delete(c.Request.Context(), id, claims.UserID, claims.Role); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleSearch handles GET /assignments (admin only).
func (h *Handler) handleSearch(c *gin.Context) {
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}

	var f SearchFilter
	f.Title = c.Query("title")
	f.Status = c.Query("status")

	courseID, ok := parseQueryUint64(c, "course_id", "INVALID_COURSE_ID", "Invalid course_id")
	if !ok {
		return
	}
	f.CourseID = courseID

	authorID, ok := parseQueryUint64(c, "author_id", "INVALID_AUTHOR_ID", "Invalid author_id")
	if !ok {
		return
	}
	f.AuthorID = authorID

	page, pageSize := parsePagination(c)
	result, err := h.service.Search(c.Request.Context(), claims.Role, f, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result.Data, "pagination": result.Pagination})
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

// parseQueryUint64 reads a named query parameter and parses it as a uint64.
// Returns (0, true) when the parameter is absent, (id, true) on success, and
// (0, false) after writing a 400 response when the value is present but invalid.
func parseQueryUint64(c *gin.Context, param, code, message string) (uint64, bool) {
	raw := c.Query(param)
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": code, "message": message},
		})
		return 0, false
	}
	return id, true
}

// parsePagination reads page and page_size query params, applying defaults of
// 1 and 20 respectively. Clamping to valid ranges is handled by the service.
func parsePagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 {
		pageSize = ps
	}
	return
}
