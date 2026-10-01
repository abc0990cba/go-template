package httpapp

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/web"
)

const indexMIME = "text/html; charset=utf-8"

// landingPage is the prerendered marketing page inside dist, served at "/".
const landingPage = "landing/index.html"

// spaFS is the built frontend: hashed assets under assets/, the SPA's
// client-router entry at index.html, and the prerendered landing under
// landing/ (all produced by `npm run build` in web/).
var spaFS, spaOpenErr = fs.Sub(web.Dist, "dist")

// serveSPA serves an asset by path and falls back to index.html for
// client-side routes, which have no file on disk. Requests that look like
// missing assets (with an extension) 404 instead of returning markup.
func serveSPA(c *gin.Context) {
	if spaOpenErr != nil {
		spaUnavailable(c)
		return
	}
	name := strings.TrimPrefix(c.Request.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(spaFS, name); err != nil {
		if isAssetPath(name) {
			c.Status(http.StatusNotFound)
			return
		}
		name = "index.html"
	}
	serveEmbedded(c, name)
}

// serveEmbedded answers with one file from the embedded dist. Hashed assets
// (assets/…) are immutable and cached for a year; everything else (HTML,
// robots.txt, sitemap.xml) revalidates through a strong ETag, matching
// 304s without a body. When the client accepts it and scripts/prerender.mjs
// left a precompressed sibling next to the file, the encoded bytes go out
// with Content-Encoding — compression without per-request CPU, which keeps
// the distroless binary self-sufficient.
func serveEmbedded(c *gin.Context, name string) {
	data, err := fs.ReadFile(spaFS, name)
	if err != nil {
		spaUnavailable(c)
		return
	}
	ctype := indexMIME
	if ext := path.Ext(name); ext != ".html" {
		ctype = mime.TypeByExtension(ext)
		if ctype == "" {
			ctype = http.DetectContentType(data[:min(len(data), 512)])
		}
	}
	c.Header("Cache-Control", cachePolicy(name))
	c.Header("Vary", "Accept-Encoding")
	etag := strongETag(data)
	c.Header("ETag", etag)
	if matchesETag(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	payload, encoding := encodedVariant(c, name, data)
	if encoding != "" {
		c.Header("Content-Encoding", encoding)
	}
	c.Data(http.StatusOK, ctype, payload)
}

// cachePolicy: content-hashed asset filenames are safe to cache forever;
// everything else must revalidate so a redeploy is picked up immediately.
func cachePolicy(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// encodedVariant picks the precompressed sibling (.br, then .gz) the client
// actually accepts, falling back to identity bytes.
func encodedVariant(c *gin.Context, name string, data []byte) ([]byte, string) {
	accept := c.GetHeader("Accept-Encoding")
	for _, variant := range []struct{ encoding, ext string }{{"br", ".br"}, {"gzip", ".gz"}} {
		if !acceptsEncoding(accept, variant.encoding) {
			continue
		}
		if encoded, err := fs.ReadFile(spaFS, name+variant.ext); err == nil {
			return encoded, variant.encoding
		}
	}
	return data, ""
}

// acceptsEncoding reports whether an Accept-Encoding header lists the
// encoding with a non-zero quality parameter.
func acceptsEncoding(header, name string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if token != name {
			continue
		}
		if q, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(params), "q=")), 64); err == nil && q <= 0 {
			continue
		}
		return true
	}
	return false
}

// strongETag fingerprints the uncompressed content; it doubles as the
// validator for every encoded variant of the same file.
func strongETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// isAssetPath reports whether the path looks like a static asset rather
// than a client-side route (which are extension-free by convention).
func isAssetPath(name string) bool {
	base := name
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	return strings.Contains(base, ".")
}

// spaUnavailable answers when the binary embeds only the placeholder dist
// (compiled before the first frontend build).
func spaUnavailable(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusServiceUnavailable,
		`<!doctype html><html><body><h1>Frontend build missing</h1><p>Run <code>make web-build</code> and rebuild the binary.</p></body></html>`)
}
