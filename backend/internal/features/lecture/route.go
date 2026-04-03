package lecture

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountLectureRoutes registers all lecture endpoints on the given router group.
//
//	POST   /courses/:courseId/lectures   – create lecture (teacher or admin)
//	GET    /courses/:courseId/lectures   – list lectures (enrolled/teacher/admin)
//	GET    /lectures/:id                 – get lecture (enrolled/teacher/admin)
//	PUT    /lectures/:id                 – update lecture (author or admin)
//	DELETE /lectures/:id                 – delete lecture (author or admin)
//	GET    /lectures                     – search all lectures (admin only)
func MountLectureRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	course := rg.Group("/courses/:courseId/lectures")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleGetByCourse)
	}

	lectures := rg.Group("/lectures")
	{
		lectures.GET("", auth, h.handleSearch)
		lectures.GET("/:id", auth, h.handleGet)
		lectures.PUT("/:id", auth, h.handleUpdate)
		lectures.DELETE("/:id", auth, h.handleDelete)
	}
}
