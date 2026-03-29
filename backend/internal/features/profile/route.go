package profile

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountProfileRoutes registers all profile endpoints under /profiles within
// the provided router group. Authentication is enforced per-route via the
// Authenticate middleware, so individual routes can be made public by simply
// omitting it.
//
//	GET    /profiles           → list all profiles          (auth required)
//	GET    /profiles/:id       → get a single profile by id (auth required)
//	POST   /profiles           → create a profile           (auth required)
//	POST   /profiles/batch     → batch-read by id list      (auth required)
//	PUT    /profiles/:id       → update username/avatar     (auth required)
//	DELETE /profiles/:id       → delete a profile           (auth required)
func MountProfileRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	profiles := rg.Group("/profiles")
	{
		profiles.GET("", auth, h.handleGetAll)
		profiles.GET("/:id", auth, h.handleGet)
		profiles.POST("", auth, h.handleCreate)
		profiles.POST("/batch", auth, h.handleBatch)
		profiles.PUT("/:id", auth, h.handleUpdate)
		profiles.DELETE("/:id", auth, h.handleDelete)
	}
}
