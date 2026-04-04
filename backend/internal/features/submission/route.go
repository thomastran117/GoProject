package submission

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountSubmissionRoutes registers all submission endpoints under
// /assignments/:id/submissions within the provided router group.
// Authentication is enforced on every route; role and ownership checks
// are handled in the service layer.
//
//	POST /assignments/:id/submissions        → submit (enrolled student or admin)
//	GET  /assignments/:id/submissions        → list all (course teacher or admin)
//	GET  /assignments/:id/submissions/mine   → get own submission (any authenticated user)
//	PUT  /assignments/:id/submissions/:subId → grade submission (course teacher or admin)
func MountSubmissionRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	subs := rg.Group("/assignments/:id/submissions")
	{
		subs.POST("", auth, h.handleSubmit)
		subs.GET("", auth, h.handleList)
		subs.GET("/mine", auth, h.handleGetMine)
		subs.PUT("/:subId", auth, h.handleGrade)
	}
}
