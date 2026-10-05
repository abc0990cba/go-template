// Package httpx holds the cross-cutting HTTP helpers every feature's
// handlers share: problem+json responses, error mapping, body binding,
// list query parsing, and session cookie handling. It imports only kernel
// and reliability — never a domain or feature package — so features may
// depend on it freely. The typed user reader (CurrentUser) lives with the
// users feature, which owns that type.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/reliability"
)

// Problem writes the RFC 9457 problem+json body.
func Problem(c *gin.Context, status int, title string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	c.JSON(status, gin.H{"type": "about:blank", "title": title, "status": status, "detail": detail})
}

// HTTPError maps kernel and reliability errors to an HTTP status and a safe
// public title. Feature-specific sentinels are mapped by their own handlers
// before falling through to Fail.
func HTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, kernel.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, kernel.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, reliability.ErrCircuitOpen):
		return http.StatusServiceUnavailable, "dependency temporarily unavailable"
	case errors.Is(err, kernel.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, kernel.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case kernel.IsValidationError(err):
		return http.StatusBadRequest, "validation failed"
	case errors.Is(err, context.DeadlineExceeded):
		// The request-timeout middleware's deadline reached a dependency;
		// the client should retry (idempotency keys make that safe).
		return http.StatusGatewayTimeout, "deadline exceeded"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// Fail writes the problem+json response for err. Internal errors are logged
// with details but their messages are never leaked to the client.
func Fail(c *gin.Context, logger *slog.Logger, err error) {
	status, title := HTTPError(err)
	if status == http.StatusInternalServerError {
		if logger != nil {
			logger.Error("request failed", "method", c.Request.Method, "path", c.Request.URL.Path, "err", err)
		}
		Problem(c, status, title, nil)
		return
	}
	Problem(c, status, title, err)
}

// ClientSafeMessage reports whether err carries a message fit to show an end
// user in the HTML UI; unexpected errors must be logged instead.
func ClientSafeMessage(err error) (string, bool) {
	switch {
	case kernel.IsValidationError(err),
		errors.Is(err, kernel.ErrNotFound),
		errors.Is(err, kernel.ErrConflict),
		errors.Is(err, kernel.ErrInvalidCredentials):
		return err.Error(), true
	default:
		return "", false
	}
}

// Bind decodes the JSON body into dst, answering 400/413 problem+json on
// failure. It reports whether the handler may continue.
func Bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			Problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
		} else {
			Problem(c, http.StatusBadRequest, "invalid request body", err)
		}
		return false
	}
	return true
}

// NextOffsetCursor renders the continuation cursor for offset-paged lists:
// a full page carries the next offset, a short page means the end (empty
// cursor). The convention every list endpoint shares.
func NextOffsetCursor(offset, batchLen int) string {
	if batchLen == 0 {
		return ""
	}
	return strconv.Itoa(offset + batchLen)
}

// ListOrEmpty keeps nil slices off the wire: a nil slice marshals to JSON
// null, and the SPA's list code expects an array everywhere.
func ListOrEmpty[T any](slice []T) []T {
	if slice == nil {
		return []T{}
	}
	return slice
}

// IntQuery parses an integer query parameter with a fallback.
func IntQuery(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, ""))
	if err != nil {
		return fallback
	}
	return value
}

// ListOptions builds the shared pagination/filter options from query
// parameters.
func ListOptions(c *gin.Context) kernel.ListOptions {
	return kernel.ListOptions{
		Query: c.Query("q"), Mode: c.DefaultQuery("mode", "offset"), Limit: IntQuery(c, "limit", 25),
		Offset: IntQuery(c, "offset", 0), After: c.Query("after"),
		Status: c.Query("status"), Type: c.Query("type"),
	}
}

// SessionToken reads the opaque session credential from the cookie or the
// Authorization header (Bearer prefix optional for scripting clients).
func SessionToken(c *gin.Context) string {
	cookie, _ := c.Cookie("app_session")
	if cookie != "" {
		return cookie
	}
	header := c.GetHeader("Authorization")
	if token, ok := strings.CutPrefix(header, "Bearer "); ok {
		return token
	}
	return header
}

// SetSession issues the session cookie with the hardening flags; the TTL
// itself is enforced server-side.
func SetSession(c *gin.Context, token string, ttl time.Duration, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_session", token, int(ttl.Seconds()), "/", "", secure, true)
}

// ClearSession expires the session cookie.
func ClearSession(c *gin.Context, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_session", "", -1, "/", "", secure, true)
}

// UserID returns the id resolved by the auth middleware.
func UserID(c *gin.Context) string {
	if id, ok := c.Get("user_id"); ok {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}

// Domain returns the active Casbin domain resolved by the org middleware,
// defaulting to the platform domain.
func Domain(c *gin.Context) string {
	if domain, ok := c.Get("domain"); ok {
		if s, ok := domain.(string); ok && s != "" {
			return s
		}
	}
	return "*"
}
