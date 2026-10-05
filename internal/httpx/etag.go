package httpx

// Optimistic concurrency: writers send If-Match with the ETag their last
// read returned; the handler compares it against the tag of the resource's
// current state before applying a change.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	ginjson "github.com/gin-gonic/gin/codec/json"
)

// ComputeETag hashes v with gin's own JSON codec — the exact bytes c.JSON
// puts on the wire — producing the same strong tag the etag middleware
// computes over a GET response body.
func ComputeETag(v any) (string, error) {
	data, err := ginjson.API.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`, nil
}

// RequireIfMatch enforces optimistic locking when the client sent If-Match:
// a mismatch answers 412 and the caller must not apply the change. A missing
// header keeps last-write-wins — clients opt in by echoing the ETag their
// GET returned, so existing scripts never break.
func RequireIfMatch(c *gin.Context, current any) bool {
	header := c.GetHeader("If-Match")
	if header == "" {
		return true
	}
	etag, err := ComputeETag(current)
	if err == nil && ifMatchMatches(header, etag) {
		return true
	}
	Problem(c, http.StatusPreconditionFailed, "resource changed since it was read", nil)
	return false
}

// ifMatchMatches compares the header against the computed tag; the header
// may carry a comma-separated list or the weak "*" comparator.
func ifMatchMatches(header, etag string) bool {
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
