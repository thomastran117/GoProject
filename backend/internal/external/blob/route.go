package blob

import (
	"backend/internal/config/middleware"

	"github.com/gin-gonic/gin"
)

// MountBlobRoutes registers blob endpoints under /blob within the provided
// router group.
//
//	POST /blob/upload-url → generate a presigned SAS upload URL (auth required)
func MountBlobRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	blob := rg.Group("/blob")
	{
		blob.POST("/upload-url", auth, h.handleGenerateUploadURL)
		blob.POST("/confirm", auth, h.handleConfirmUpload)
	}
}
