package lecture

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type lectureService interface {
	Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*LectureResponse, error)
	GetByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, page, pageSize int) (*PagedResult, error)
	GetByID(ctx context.Context, callerUserID uint64, callerRole string, id uint64) (*LectureResponse, error)
	Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*LectureResponse, error)
	Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error
	Search(ctx context.Context, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error)
}

type createLectureRequest struct {
	Title   string `json:"title"   binding:"required,min=1,max=300"`
	Content string `json:"content" binding:"required,min=1"`
}

type updateLectureRequest struct {
	Title   string `json:"title"   binding:"required,min=1,max=300"`
	Content string `json:"content" binding:"required,min=1"`
}

// Handler holds the HTTP handlers for the lecture resource.
type Handler struct {
	service lectureService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleCreate handles POST /courses/:courseId/lectures.
func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createLectureRequest
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
	result, err := h.service.Create(c.Request.Context(), claims.UserID, claims.Role, courseID, CreateParams{
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleGetByCourse handles GET /courses/:courseId/lectures.
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

// handleGet handles GET /lectures/:id.
func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid lecture ID")
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

// handleUpdate handles PUT /lectures/:id.
func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid lecture ID")
	if err != nil {
		return
	}
	var req updateLectureRequest
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
	result, err := h.service.Update(c.Request.Context(), id, claims.UserID, claims.Role, UpdateParams{
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDelete handles DELETE /lectures/:id.
func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid lecture ID")
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

// handleSearch handles GET /lectures (admin only).
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
// 1 and 20 respectively.
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
