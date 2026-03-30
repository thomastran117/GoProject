package school

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountSchoolRoutes registers all school endpoints under /schools within the
// provided router group. Authentication is enforced on every route; role and
// ownership checks are handled in the service layer.
//
//	GET    /schools          → list all schools                     (auth required)
//	GET    /schools/search   → filter by name/city/country/principal (auth required)
//	GET    /schools/:id      → get a single school by id            (auth required)
//	POST   /schools          → create a school                      (principal only)
//	POST   /schools/batch    → batch-read by id list                (auth required)
//	PUT    /schools/:id      → update a school                      (owning principal only)
//	DELETE /schools/:id      → delete a school                      (owning principal only)
func MountSchoolRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	schools := rg.Group("/schools")
	{
		schools.GET("", auth, h.handleGetAll)
		schools.GET("/search", auth, h.handleSearch)
		schools.GET("/:id", auth, h.handleGet)
		schools.POST("", auth, h.handleCreate)
		schools.POST("/batch", auth, h.handleBatch)
		schools.PUT("/:id", auth, h.handleUpdate)
		schools.DELETE("/:id", auth, h.handleDelete)
	}
}
