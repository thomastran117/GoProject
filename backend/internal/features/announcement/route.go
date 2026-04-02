package announcement

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountAnnouncementRoutes registers all announcement endpoints within the
// provided router group. Authentication is enforced on every route; role and
// ownership checks are handled in the service layer.
//
// Course-scoped routes (nested under /courses/:courseId/announcements):
//
//	POST /courses/:courseId/announcements → create announcement (teacher of course or admin)
//	GET  /courses/:courseId/announcements → list announcements for a course, paginated (auth required)
//
// Standalone announcement routes:
//
//	GET    /announcements        → search all announcements with pagination (admin only)
//	GET    /announcements/:id    → get a single announcement (auth required)
//	PUT    /announcements/:id    → update an announcement (author or admin)
//	DELETE /announcements/:id    → delete an announcement (author or admin)
func MountAnnouncementRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	// Course-scoped announcement routes
	course := rg.Group("/courses/:courseId/announcements")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleGetByCourse)
	}

	// Standalone announcement routes
	announcements := rg.Group("/announcements")
	{
		announcements.GET("", auth, h.handleSearch)
		announcements.GET("/:id", auth, h.handleGet)
		announcements.PUT("/:id", auth, h.handleUpdate)
		announcements.DELETE("/:id", auth, h.handleDelete)
	}
}
