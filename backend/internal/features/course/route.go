package course

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountCourseRoutes registers all course endpoints under /courses within the
// provided router group. Authentication is enforced on every route; role and
// ownership checks are handled in the service layer.
//
//	GET    /courses/search  → filter by name/code/school/teacher/subject/etc  (auth required)
//	GET    /courses/:id     → get a single course by id                        (auth required)
//	POST   /courses         → create a course                                  (principal or admin only)
//	POST   /courses/batch   → batch-read by id list                            (auth required)
//	PUT    /courses/:id     → update a course                                  (principal/teacher/admin)
//	DELETE /courses/:id     → delete a course                                  (principal or admin only)
func MountCourseRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	courses := rg.Group("/courses")
	{
		courses.GET("/search", auth, h.handleSearch)
		courses.GET("/:id", auth, h.handleGet)
		courses.POST("", auth, h.handleCreate)
		courses.POST("/batch", auth, h.handleBatch)
		courses.PUT("/:id", auth, h.handleUpdate)
		courses.DELETE("/:id", auth, h.handleDelete)
	}
}
