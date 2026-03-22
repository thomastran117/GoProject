package middleware

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Message
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Written() {
			return
		}

		errs := c.Errors
		if len(errs) == 0 {
			return
		}

		err := errs.Last().Err

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			c.JSON(apiErr.Status, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apiErr.Code,
					"message": apiErr.Message,
				},
			})
			return
		}

		log.Printf("unhandled error: %v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": "Something went wrong",
			},
		})
	}
}
