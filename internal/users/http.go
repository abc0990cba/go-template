// Users HTTP surface: admin CRUD over accounts. Self-service and auth-mixed
// routes (/me, /users/:id/activity, /users/:id/mfa, /users/:id/roles,
// /users/me/export, /users/:id/enrich-demo) live with their own features.
package users

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
)

// HTTP carries the user handlers' dependencies.
type HTTP struct {
	Svc *Service
	// Audit records the mutation trail. A func, not *audit.Service: audit
	// reads the users.User actor out of the request context, so users
	// importing audit back would cycle the graph. The assembly passes a
	// closure over audit.Record.
	Audit  func(c *gin.Context, action, resourceType, resourceID string, before, after any)
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
	// ETag adds strong validation to the heavy list/get GETs.
	ETag func() gin.HandlerFunc
}

// RegisterHTTP mounts the user routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/users", h.RequirePermission("users", "read"), h.ETag(), h.list)
	authorized.POST("/users", h.RequirePermission("users", "create"), h.create)
	authorized.GET("/users/:id", h.RequirePermission("users", "read"), h.ETag(), h.get)
	authorized.PUT("/users/:id", h.RequirePermission("users", "update"), h.update)
	authorized.DELETE("/users/:id", h.RequirePermission("users", "delete"), h.remove)
}

// CurrentUser returns the user the auth middleware resolved into the request
// context.
func CurrentUser(c *gin.Context) User {
	user, _ := c.Get("user")
	return user.(User)
}

// CurrentUserOK is CurrentUser for routes where no one may be authenticated
// yet (register, login): it reports whether a user was resolved.
func CurrentUserOK(c *gin.Context) (User, bool) {
	v, ok := c.Get("user")
	if !ok {
		return User{}, false
	}
	user, ok := v.(User)
	return user, ok
}

// createUserRequest mirrors the generated CreateUserRequest: email required,
// name and external_ref optional.
type createUserRequest struct {
	Email       string  `json:"email"`
	ExternalRef *string `json:"external_ref,omitempty"`
	Name        *string `json:"name,omitempty"`
}

// updateUserRequest mirrors the generated UpdateUserRequest: name required,
// external_ref optional.
type updateUserRequest struct {
	Email       string  `json:"email"`
	ExternalRef *string `json:"external_ref,omitempty"`
	Name        string  `json:"name"`
}

// derefString normalizes the generated optional fields.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// list pages through accounts in the requested pagination mode.
func (h *HTTP) list(c *gin.Context) {
	users, next, err := h.Svc.List(c.Request.Context(), httpx.ListOptions(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(users), "next_cursor": next})
}

func (h *HTTP) create(c *gin.Context) {
	var req createUserRequest
	if !httpx.Bind(c, &req) {
		return
	}
	user, err := h.Svc.Create(c.Request.Context(), req.Email, derefString(req.Name), derefString(req.ExternalRef))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	h.Audit(c, "user.created", "user", user.ID, nil, user)
	c.JSON(http.StatusCreated, user)
}

func (h *HTTP) get(c *gin.Context) {
	user, err := h.Svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *HTTP) update(c *gin.Context) {
	var req updateUserRequest
	if !httpx.Bind(c, &req) {
		return
	}
	before, _ := h.Svc.Get(c.Request.Context(), c.Param("id"))
	user, err := h.Svc.Update(c.Request.Context(), c.Param("id"), req.Email, req.Name, derefString(req.ExternalRef))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	h.Audit(c, "user.updated", "user", user.ID, before, user)
	c.JSON(http.StatusOK, user)
}

func (h *HTTP) remove(c *gin.Context) {
	before, _ := h.Svc.Get(c.Request.Context(), c.Param("id"))
	if err := h.Svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	h.Audit(c, "user.deleted", "user", c.Param("id"), before, nil)
	c.Status(http.StatusNoContent)
}
