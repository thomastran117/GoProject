package activitylog

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

func Mount(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	logs := rg.Group("/activity-logs")
	logs.POST("/page-view",  auth, h.handlePageView)
	logs.GET("/requests",    auth, h.handleRequestStats)
	logs.GET("/page-views",  auth, h.handlePageViewStats)
	logs.GET("/summary",     auth, h.handleSummary)
}
