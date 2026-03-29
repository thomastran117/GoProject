package request

import (
	"net/http"

	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

func BindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(&middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
		})
		return false
	}
	return true
}