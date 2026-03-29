package middleware

import (
	"log"
	"net/http"
	"strings"

	token "backend/internal/features/token"

	"github.com/gin-gonic/gin"
)

const claimsKey = "auth_claims"

// Authenticate is a Gin middleware that validates the Bearer token in the
// Authorization header. On success it stores the parsed claims in the context
// so handlers can retrieve them with GetClaims. On failure it aborts with 401.
func Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_TOKEN",
					"message": "Authorization header missing or malformed",
				},
			})
			return
		}

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

// extractBearerToken parses the Authorization header using strings.Fields so
// that extra whitespace and case variations on "bearer" are handled robustly.
// Returns the token string and true only when the scheme is "bearer" and the
// token value is non-empty.
func extractBearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") {
		return "", false
	}
	return fields[1], true
}

// GetClaims retrieves the JWT claims stored by the Authenticate middleware.
// Returns false if the middleware was not applied to this route.
// Logs an internal error if the stored value has an unexpected type, which
// would indicate a programming mistake rather than a client error.
func GetClaims(c *gin.Context) (*token.AccessClaims, bool) {
	value, exists := c.Get(claimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*token.AccessClaims)
	if !ok {
		log.Printf("middleware: claimsKey held unexpected type %T", value)
		return nil, false
	}
	return claims, ok
}
