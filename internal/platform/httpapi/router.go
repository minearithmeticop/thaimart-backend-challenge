// Package httpapi is the HTTP adapter (a "driving" adapter in hexagonal
// terms): it translates HTTP requests into use-case calls and use-case
// errors into HTTP responses. It never talks to MongoDB directly — only
// through ports declared in internal/app.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
)

// NewRouter builds the gin engine with all routes wired.
func NewRouter(health app.HealthChecker) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := health.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}
