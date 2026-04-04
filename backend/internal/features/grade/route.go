package grade

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountGradeRoutes registers all grade endpoints under /courses/:id/grades
// within the provided router group. Authentication is enforced on every route;
// role and ownership checks are handled in the service layer.
//
//	POST   /courses/:id/grades           → create grade (teacher/admin)
//	GET    /courses/:id/grades           → list all students' grades (teacher/admin)
//	GET    /courses/:id/grades/mine      → own grades (enrolled student, teacher, admin)
//	PUT    /courses/:id/grades/:gradeId  → update grade (teacher/admin)
//	DELETE /courses/:id/grades/:gradeId  → delete grade (teacher/admin)
func MountGradeRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()
	grades := rg.Group("/courses/:id/grades")
	{
		grades.POST("", auth, h.handleCreate)
		grades.GET("", auth, h.handleListAll)
		grades.GET("/mine", auth, h.handleGetMine)
		grades.PUT("/:gradeId", auth, h.handleUpdate)
		grades.DELETE("/:gradeId", auth, h.handleDelete)
	}
}
