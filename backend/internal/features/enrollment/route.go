package enrollment

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountEnrollmentRoutes registers all enrollment and invite endpoints.
//
// Course-scoped:
//
//	POST   /courses/:courseId/enroll       — enroll self (any authenticated user)
//	DELETE /courses/:courseId/enroll       — unenroll self (any authenticated user)
//	GET    /courses/:courseId/enrollments  — list enrolled users (teacher/principal/admin)
//	POST   /courses/:courseId/invites      — create invite (teacher/principal/admin)
//	GET    /courses/:courseId/invites      — list invites (teacher/principal/admin)
//
// Invite-scoped:
//
//	POST   /invites/:code/accept           — accept invite (invitee only)
//
// User-scoped:
//
//	GET    /users/me/enrollments           — list my enrollments (any authenticated user)
func MountEnrollmentRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	courses := rg.Group("/courses/:courseId")
	courses.POST("/enroll", auth, h.handleEnroll)
	courses.DELETE("/enroll", auth, h.handleUnenroll)
	courses.GET("/enrollments", auth, h.handleGetEnrollmentsByCourse)
	courses.POST("/invites", auth, h.handleCreateInvite)
	courses.GET("/invites", auth, h.handleGetInvitesByCourse)

	rg.POST("/invites/:code/accept", auth, h.handleAcceptInvite)
	rg.GET("/users/me/enrollments", auth, h.handleGetMyEnrollments)
}
