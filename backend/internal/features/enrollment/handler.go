package enrollment

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type enrollmentService interface {
	Enroll(ctx context.Context, callerUserID uint64, courseID uint64) (*EnrollmentResponse, error)
	Unenroll(ctx context.Context, callerUserID uint64, courseID uint64) error
	GetEnrollmentsByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, page, pageSize int) (*PagedEnrollments, error)
	GetMyEnrollments(ctx context.Context, callerUserID uint64, page, pageSize int) (*PagedEnrollments, error)
	CreateInvite(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, inviteeID uint64) (*CourseInviteResponse, error)
	AcceptInvite(ctx context.Context, callerUserID uint64, code string) (*EnrollmentResponse, error)
	GetInvitesByCourse(ctx context.Context, callerUserID uint64, callerRole string, courseID uint64, statusFilter string) ([]*CourseInviteResponse, error)
}

type createInviteRequest struct {
	InviteeID uint64 `json:"invitee_id" binding:"required"`
}

// Handler holds the HTTP handlers for enrollment and invite resources.
type Handler struct {
	service enrollmentService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleEnroll handles POST /courses/:courseId/enroll.
// Any authenticated user may enroll in a public course directly. Private
// courses require a pre-accepted invite (enforced in the service layer).
func (h *Handler) handleEnroll(c *gin.Context) {
	courseID, ok := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if !ok {
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
	enrollment, err := h.service.Enroll(c.Request.Context(), claims.UserID, courseID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": enrollment})
}

// handleUnenroll handles DELETE /courses/:courseId/enroll.
func (h *Handler) handleUnenroll(c *gin.Context) {
	courseID, ok := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if !ok {
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
	if err := h.service.Unenroll(c.Request.Context(), claims.UserID, courseID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleGetEnrollmentsByCourse handles GET /courses/:courseId/enrollments.
func (h *Handler) handleGetEnrollmentsByCourse(c *gin.Context) {
	courseID, ok := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if !ok {
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
	result, err := h.service.GetEnrollmentsByCourse(c.Request.Context(), claims.UserID, claims.Role, courseID, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result.Data, "pagination": result.Pagination})
}

// handleGetMyEnrollments handles GET /users/me/enrollments.
func (h *Handler) handleGetMyEnrollments(c *gin.Context) {
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	page, pageSize := parsePagination(c)
	result, err := h.service.GetMyEnrollments(c.Request.Context(), claims.UserID, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result.Data, "pagination": result.Pagination})
}

// handleCreateInvite handles POST /courses/:courseId/invites.
func (h *Handler) handleCreateInvite(c *gin.Context) {
	courseID, ok := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if !ok {
		return
	}
	var req createInviteRequest
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
	invite, err := h.service.CreateInvite(c.Request.Context(), claims.UserID, claims.Role, courseID, req.InviteeID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": invite})
}

// handleAcceptInvite handles POST /invites/:code/accept.
func (h *Handler) handleAcceptInvite(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_CODE", "message": "Invalid invite code"},
		})
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
	enrollment, err := h.service.AcceptInvite(c.Request.Context(), claims.UserID, code)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": enrollment})
}

// handleGetInvitesByCourse handles GET /courses/:courseId/invites.
// Accepts an optional ?status= query param to filter by invite status.
func (h *Handler) handleGetInvitesByCourse(c *gin.Context) {
	courseID, ok := parseURLUint64(c, "courseId", "INVALID_COURSE_ID", "Invalid course ID")
	if !ok {
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
	statusFilter := c.Query("status")
	invites, err := h.service.GetInvitesByCourse(c.Request.Context(), claims.UserID, claims.Role, courseID, statusFilter)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": invites})
}

// ── Parsing helpers ───────────────────────────────────────────────────────────

// parseURLUint64 reads a named URL parameter and parses it as uint64.
// On failure it writes a 400 response and returns (0, false).
func parseURLUint64(c *gin.Context, param, code, message string) (uint64, bool) {
	raw := c.Param(param)
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

// parsePagination reads page and page_size query params.
// Defaults: page=1, page_size=20. page_size is clamped to a maximum of 100.
func parsePagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if raw := c.Query("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			page = v
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 100 {
				v = 100
			}
			pageSize = v
		}
	}
	return
}
