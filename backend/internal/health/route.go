package health

import (
	"github.com/gin-gonic/gin"
)

func MountHealthRoutes(rg *gin.RouterGroup) {
	rg.GET("/ping", HandlePing)
	rg.GET("/health", HandleHealth)
	rg.GET("/", HandleHealth)
}