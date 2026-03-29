package auth

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

func MountAuthRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := rg.Group("/auth")
	{
		auth.POST("/login", h.HandleLogin)
		auth.POST("/signup", h.HandleSignup)
		auth.POST("/verify", h.HandleVerify)
		auth.POST("/google", h.HandleGoogle)
		auth.POST("/microsoft", h.HandleMicrosoft)
		auth.POST("/refresh", h.HandleRefresh)
		auth.POST("/role", middleware.Authenticate(), h.HandleSetRole)
	}
}
