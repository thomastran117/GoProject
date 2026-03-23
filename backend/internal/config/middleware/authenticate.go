package middleware

import (
	"net/http"
	"strings"

	"backend/internal/app/core/token"

	"github.com/gin-gonic/gin"
)

const claimsKey = "auth_claims"

// Authenticate is a Gin middleware that validates the Bearer token in the
// Authorization header. On success it stores the parsed claims in the context
// so handlers can retrieve them with GetClaims. On failure it aborts with 401.
func Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_TOKEN",
					"message": "Authorization header missing or malformed",
				},
			})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := token.ValidateAccess(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_TOKEN",
					"message": "Token is invalid or has expired",
				},
			})
			return
		}

		c.Set(claimsKey, claims)
		c.Next()
	}
}

// GetClaims retrieves the JWT claims stored by the Authenticate middleware.
// Returns false if the middleware was not applied to this route.
func GetClaims(c *gin.Context) (*token.AccessClaims, bool) {
	value, exists := c.Get(claimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*token.AccessClaims)
	return claims, ok
}
