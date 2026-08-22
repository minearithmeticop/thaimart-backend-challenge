package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// ---- Request bodies ----------------------------------------------------------

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// updateRequest uses pointers so absent fields stay untouched (PATCH).
type updateRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// ---- Response bodies ---------------------------------------------------------

// userResponse is the public shape of a user; the password hash is never
// serialized.
type userResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

type listUsersResponse struct {
	Users  []userResponse `json:"users"`
	Total  int64          `json:"total"`
	Limit  int64          `json:"limit"`
	Offset int64          `json:"offset"`
}

type loginResponse struct {
	Token     string       `json:"token"`
	TokenType string       `json:"token_type"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      userResponse `json:"user"`
}

// ---- Error mapping -----------------------------------------------------------

// domainStatus maps domain sentinel errors onto HTTP status codes — the
// single place where business failures become HTTP semantics.
func domainStatus(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrInvalidUserID):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, "user not found"
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return http.StatusConflict, "email already exists"
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid email or password"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "invalid or expired token"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// respondError maps err and writes the response, logging the underlying
// cause of unexpected 500s (which is never sent to the client).
func respondError(c *gin.Context, err error) {
	status, msg := domainStatus(err)
	if status >= 500 {
		slog.Error("request failed", "err", err)
	}
	c.JSON(status, gin.H{"error": msg})
}
