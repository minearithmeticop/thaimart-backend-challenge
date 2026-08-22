package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/auth"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// handler translates HTTP into use-case calls and use-case errors into
// responses. It depends on use cases, never on repositories.
type handler struct {
	auth  *auth.Service
	users *user.Service
}

func newHandler(authSvc *auth.Service, users *user.Service) *handler {
	return &handler{auth: authSvc, users: users}
}

// register handles POST /api/v1/auth/register (public).
func (h *handler) register(c *gin.Context) {
	var req registerRequest
	if err := bindJSON(c, &req); err != nil {
		respondError(c, err)
		return
	}
	u, err := h.auth.Register(c.Request.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Location", "/api/v1/users/"+u.ID)
	c.JSON(http.StatusCreated, toUserResponse(u))
}

// login handles POST /api/v1/auth/login (public) and returns a signed JWT.
func (h *handler) login(c *gin.Context) {
	var req loginRequest
	if err := bindJSON(c, &req); err != nil {
		respondError(c, err)
		return
	}
	session, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		Token:     session.Token,
		TokenType: "Bearer",
		ExpiresAt: session.ExpiresAt,
		User:      toUserResponse(session.User),
	})
}

// createUser handles POST /api/v1/users (JWT-protected).
func (h *handler) createUser(c *gin.Context) {
	var req registerRequest
	if err := bindJSON(c, &req); err != nil {
		respondError(c, err)
		return
	}
	u, err := h.users.Create(c.Request.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Location", "/api/v1/users/"+u.ID)
	c.JSON(http.StatusCreated, toUserResponse(u))
}

// getUser handles GET /api/v1/users/:id (JWT-protected).
func (h *handler) getUser(c *gin.Context) {
	u, err := h.users.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(u))
}

// listUsers handles GET /api/v1/users?limit=&offset= (JWT-protected).
func (h *handler) listUsers(c *gin.Context) {
	limit, err := queryInt64(c, "limit")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
		return
	}
	offset, err := queryInt64(c, "offset")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be an integer"})
		return
	}

	users, total, err := h.users.List(c.Request.Context(), limit, offset)
	if err != nil {
		respondError(c, err)
		return
	}

	resp := listUsersResponse{
		Users:  make([]userResponse, len(users)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if resp.Limit == 0 {
		resp.Limit = user.DefaultPageSize // reflect the default the service applied
	}
	for i, u := range users {
		resp.Users[i] = toUserResponse(u)
	}
	c.JSON(http.StatusOK, resp)
}

// updateUser handles PATCH /api/v1/users/:id (JWT-protected).
func (h *handler) updateUser(c *gin.Context) {
	var req updateRequest
	if err := bindJSON(c, &req); err != nil {
		respondError(c, err)
		return
	}
	u, err := h.users.Update(c.Request.Context(), c.Param("id"), user.Update{
		Name:  req.Name,
		Email: req.Email,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(u))
}

// deleteUser handles DELETE /api/v1/users/:id (JWT-protected).
func (h *handler) deleteUser(c *gin.Context) {
	if err := h.users.Delete(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// bindJSON decodes a JSON body. An empty body is treated as an empty
// object so partial-update semantics stay in the use case; anything else
// that fails to parse is invalid input.
func bindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("%w: request body must be valid JSON", domain.ErrInvalidInput)
	}
	return nil
}

func queryInt64(c *gin.Context, key string) (int64, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
