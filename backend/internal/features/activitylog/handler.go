package activitylog

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/application/request"
	"backend/internal/features/token"

	"github.com/gin-gonic/gin"
)

type activityLogService interface {
	RecordPageView(ctx context.Context, callerRole string, p RecordPageViewParams) error
	GetRequestStats(ctx context.Context, callerRole string, from, to time.Time, filterRole string) ([]*RequestStatsResponse, error)
	GetPageViewStats(ctx context.Context, callerRole string, from, to time.Time, filterRole string) ([]*PageViewStatsResponse, error)
	GetSummary(ctx context.Context, callerRole string) (*SummaryResponse, error)
}

type Handler struct {
	service activityLogService
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
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

type pageViewRequest struct {
	Page       string `json:"page"        binding:"required,min=1,max=500"`
	DurationMs int    `json:"duration_ms" binding:"min=0"`
}

func (h *Handler) handlePageView(c *gin.Context) {
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	var req pageViewRequest
	if !request.BindJSON(c, &req) {
		return
	}
	if err := h.service.RecordPageView(c.Request.Context(), claims.Role, RecordPageViewParams{
		Page:       req.Page,
		DurationMs: req.DurationMs,
	}); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (h *Handler) handleRequestStats(c *gin.Context) {
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	from, to, ok := parseDateRange(c)
	if !ok {
		return
	}
	stats, err := h.service.GetRequestStats(c.Request.Context(), claims.Role, from, to, c.Query("role"))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

func (h *Handler) handlePageViewStats(c *gin.Context) {
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	from, to, ok := parseDateRange(c)
	if !ok {
		return
	}
	stats, err := h.service.GetPageViewStats(c.Request.Context(), claims.Role, from, to, c.Query("role"))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

func (h *Handler) handleSummary(c *gin.Context) {
	claims, ok := getClaims(c)
	if !ok {
		return
	}
	summary, err := h.service.GetSummary(c.Request.Context(), claims.Role)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func parseDateRange(c *gin.Context) (from, to time.Time, ok bool) {
	now := time.Now().UTC()
	from = now.AddDate(0, 0, -7)
	to = now

	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   gin.H{"code": "INVALID_DATE", "message": "from must be RFC3339"},
			})
			return time.Time{}, time.Time{}, false
		}
		from = t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   gin.H{"code": "INVALID_DATE", "message": "to must be RFC3339"},
			})
			return time.Time{}, time.Time{}, false
		}
		to = t
	}
	return from, to, true
}
