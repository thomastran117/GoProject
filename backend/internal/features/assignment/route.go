package assignment

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountAssignmentRoutes registers all assignment endpoints within the provided
// router group. Authentication is enforced on every route; role and ownership
// checks are handled in the service layer.
//
// Course-scoped routes (nested under /courses/:courseId/assignments):
//
//	POST /courses/:courseId/assignments → create assignment (teacher of course or admin)
//	GET  /courses/:courseId/assignments → list assignments for a course, paginated (auth required)
//
// Standalone assignment routes:
//
//	GET    /assignments        → search all assignments with pagination (admin only)
//	GET    /assignments/:id    → get a single assignment (auth required)
//	PUT    /assignments/:id    → update an assignment (author or admin)
//	DELETE /assignments/:id    → delete an assignment (author or admin)
func MountAssignmentRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	// Course-scoped assignment routes
	course := rg.Group("/courses/:courseId/assignments")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleGetByCourse)
	}

	// Standalone assignment routes
	assignments := rg.Group("/assignments")
	{
		assignments.GET("", auth, h.handleSearch)
		assignments.GET("/:id", auth, h.handleGet)
		assignments.PUT("/:id", auth, h.handleUpdate)
		assignments.DELETE("/:id", auth, h.handleDelete)
	}
}
