package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
)

// withLogging writes one structured log line per request: method, path,
// status and duration.
func withLogging(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
			"remote", c.ClientIP(),
		)
	}
}

const claimsContextKey = "auth.claims"

// withAuth guards a route behind a valid bearer token. On success the
// verified claims are stored in the gin context for handlers to read.
func withAuth(tokens app.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header, expected: Bearer <token>",
			})
			return
		}
		claims, err := tokens.Verify(strings.TrimSpace(token))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set(claimsContextKey, claims)
		c.Next()
	}
}
