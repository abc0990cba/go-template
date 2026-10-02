package httpapp

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxETagBuffer bounds the responses the middleware is willing to buffer:
// list endpoints clamp their page size, so real bodies sit far below this.
const maxETagBuffer = 2 << 20

// etagWriter holds the response body while the handler runs, switching to
// pass-through if it grows past the buffer budget (the tag is then skipped).
type etagWriter struct {
	gin.ResponseWriter
	buf  []byte
	full bool
}

func (w *etagWriter) Write(b []byte) (int, error) {
	if !w.full {
		if len(w.buf)+len(b) > maxETagBuffer {
			// Too large to validate: flush what we held and stream the rest.
			w.full = true
			if len(w.buf) > 0 {
				held := w.buf
				w.buf = nil
				if _, err := w.ResponseWriter.Write(held); err != nil {
					return 0, err
				}
			}
		} else {
			w.buf = append(w.buf, b...)
			return len(b), nil
		}
	}
	return w.ResponseWriter.Write(b)
}

// etag adds strong ETag validation to heavy GET endpoints: an If-None-Match
// match answers 304 without a body, saving the wire cost of dashboard
// polling. Applied per route, after the permission guards.
func (a *App) etag() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		writer := &etagWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()

		if writer.full {
			// Oversized body: bytes were already streamed through.
			return
		}
		if writer.Status() == http.StatusOK && len(writer.buf) > 0 {
			sum := sha256.Sum256(writer.buf)
			etag := `"` + hex.EncodeToString(sum[:16]) + `"`
			if matchesETag(c.GetHeader("If-None-Match"), etag) {
				writer.Header().Del("Content-Length")
				writer.WriteHeader(http.StatusNotModified)
				return
			}
			writer.Header().Set("ETag", etag)
		}
		// Everything else (errors, redirects, empty 200s) passes through
		// exactly as the handler wrote it.
		if len(writer.buf) > 0 {
			_, _ = writer.ResponseWriter.Write(writer.buf)
		}
	}
}

// matchesETag compares the header against the computed tag; the header may
// carry a comma-separated list or the weak "*" comparator.
func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
