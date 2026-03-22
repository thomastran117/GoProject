package auth

import (
	"github.com/gin-gonic/gin"
)

func MountAuthRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := rg.Group("/auth")
	{
		auth.POST("/login", h.HandleLogin)
		auth.POST("/signup", h.HandleSignup)
		auth.POST("/verify", HandleVerify)
		auth.POST("/google", HandleGoogle)
		auth.POST("/microsoft", HandleMicrosoft)
		auth.POST("/refresh", HandleRefresh)
	}
}
