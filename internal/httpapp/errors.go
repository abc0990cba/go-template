package httpapp

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// httpError maps domain errors to an HTTP status and a safe public title.
func httpError(err error) (int, string) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, repository.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, service.ErrInvalidTransition):
		return http.StatusConflict, "invalid transition"
	case errors.Is(err, service.ErrAlreadyReplayed):
		return http.StatusConflict, "already replayed"
	case errors.Is(err, reliability.ErrCircuitOpen):
		return http.StatusServiceUnavailable, "dependency temporarily unavailable"
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case service.IsValidationError(err):
		return http.StatusBadRequest, "validation failed"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// fail writes the problem+json response for err. Internal errors are logged
// with details but their messages are never leaked to the client.
func (a *App) fail(c *gin.Context, err error) {
	status, title := httpError(err)
	if status == http.StatusInternalServerError {
		a.logger.Error("request failed", "method", c.Request.Method, "path", c.Request.URL.Path, "err", err)
		a.problem(c, status, title, nil)
		return
	}
	a.problem(c, status, title, err)
}

// clientSafeMessage reports whether err carries a message fit to show an end
// user in the HTML UI; unexpected errors must be logged instead.
func clientSafeMessage(err error) (string, bool) {
	switch {
	case service.IsValidationError(err),
		errors.Is(err, repository.ErrNotFound),
		errors.Is(err, repository.ErrConflict),
		errors.Is(err, service.ErrInvalidCredentials):
		return err.Error(), true
	default:
		return "", false
	}
}
