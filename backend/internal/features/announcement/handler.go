package announcement

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type announcementService interface {
	Create(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, p CreateParams) (*AnnouncementResponse, error)
	GetByCourse(ctx context.Context, courseID uint64, page, pageSize int) (*PagedResult, error)
	GetByID(ctx context.Context, id uint64) (*AnnouncementResponse, error)
	Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*AnnouncementResponse, error)
	Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error
	Search(ctx context.Context, callerRole string, f SearchFilter, page, pageSize int) (*PagedResult, error)
}

type createAnnouncementRequest struct {
	Title       string     `json:"title"        binding:"required,min=1,max=300"`
	Body        string     `json:"body"         binding:"required,min=1"`
	Priority    string     `json:"priority"     binding:"omitempty,max=20"`
	IsPinned    bool       `json:"is_pinned"`
	PublishedAt *time.Time `json:"published_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type updateAnnouncementRequest struct {
	Title       string     `json:"title"        binding:"required,min=1,max=300"`
	Body        string     `json:"body"         binding:"required,min=1"`
	Priority    string     `json:"priority"     binding:"omitempty,max=20"`
	IsPinned    bool       `json:"is_pinned"`
	PublishedAt *time.Time `json:"published_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// Handler holds the HTTP handlers for the announcement resource.
type Handler struct {
	service announcementService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleCreate handles POST /courses/:courseId/announcements.
func (h *Handler) handleCreate(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	var req createAnnouncementRequest
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
		Body:        req.Body,
		Priority:    req.Priority,
		IsPinned:    req.IsPinned,
		PublishedAt: req.PublishedAt,
		ExpiresAt:   req.ExpiresAt,
	}
	result, err := h.service.Create(c.Request.Context(), claims.UserID, claims.Role, courseID, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

// handleGetByCourse handles GET /courses/:courseId/announcements.
func (h *Handler) handleGetByCourse(c *gin.Context) {
	courseID, err := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if err != nil {
		return
	}
	page, pageSize := parsePagination(c)
	result, err := h.service.GetByCourse(c.Request.Context(), courseID, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result.Data, "pagination": result.Pagination})
}

// handleGet handles GET /announcements/:id.
func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid announcement ID")
	if err != nil {
		return
	}
	result, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleUpdate handles PUT /announcements/:id.
func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid announcement ID")
	if err != nil {
		return
	}
	var req updateAnnouncementRequest
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
		Body:        req.Body,
		Priority:    req.Priority,
		IsPinned:    req.IsPinned,
		PublishedAt: req.PublishedAt,
		ExpiresAt:   req.ExpiresAt,
	}
	result, err := h.service.Update(c.Request.Context(), id, claims.UserID, claims.Role, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// handleDelete handles DELETE /announcements/:id.
func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseURLUint64(c, "id", "INVALID_ID", "Invalid announcement ID")
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

// handleSearch handles GET /announcements (admin only).
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
	f.Priority = c.Query("priority")

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
