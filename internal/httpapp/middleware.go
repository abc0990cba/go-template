package httpapp

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxBodyBytes = 1 << 20

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// requestID tags every request with a collision-resistant id, honoring an
// upstream X-Request-ID so the id survives proxy hops.
func (a *App) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Next()
	}
}

// bodyLimit caps request bodies before handlers read them. The
// ContentLength check rejects pre-declared oversize bodies up front; the
// MaxBytesReader wrapper catches chunked requests whose size only becomes
// known while streaming — the mid-read overflow surfaces in a.bind as 413.
func (a *App) bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBodyBytes {
			a.problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
			c.Abort()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}
		c.Next()
	}
}

// bind decodes the JSON request body into dst and writes the error response
// itself: 413 problem+json when the body exceeded the cap mid-read (the
// ContentLength pre-check cannot see chunked framing), 400 otherwise. It
// reports whether decoding succeeded.
func (a *App) bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			a.problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
		} else {
			a.problem(c, http.StatusBadRequest, "invalid request body", err)
		}
		return false
	}
	return true
}
