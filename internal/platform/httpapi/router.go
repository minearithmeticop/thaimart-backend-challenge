// Package httpapi is the HTTP adapter (a "driving" adapter in hexagonal
// terms): it translates HTTP requests into use-case calls and use-case
// errors into HTTP responses. It never talks to MongoDB directly — only
// through ports declared in internal/app.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/auth"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
)

// Route patterns shared between the method-specific registrations.
const (
	routeUsers    = "/users"
	routeUserByID = "/users/:id"
)

// NewRouter builds the gin engine with all routes wired. Public routes:
// health, register, login. Everything else requires a valid bearer token.
func NewRouter(
	health app.HealthChecker,
	tokens app.TokenManager,
	authSvc *auth.Service,
	users *user.Service,
	log *slog.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(withLogging(log))

	h := newHandler(authSvc, users)

	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := health.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.POST("/register", h.register)
		authGroup.POST("/login", h.login)
	}

	api := r.Group("/api/v1", withAuth(tokens))
	{
		api.POST(routeUsers, h.createUser)
		api.GET(routeUsers, h.listUsers)
		api.GET(routeUserByID, h.getUser)
		api.PATCH(routeUserByID, h.updateUser)
		api.DELETE(routeUserByID, h.deleteUser)
	}

	return r
}
