package httpapp

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/web"
)

const indexMIME = "text/html; charset=utf-8"

// spaFS is the built frontend; assets are hashed filenames, index.html is
// the client-side router's entry point.
var spaFS, spaOpenErr = fs.Sub(web.Dist, "dist")

// serveSPA serves an asset by path and falls back to index.html for
// client-side routes, which have no file on disk. Requests that look like
// missing assets (with an extension) 404 instead of returning markup.
func serveSPA(c *gin.Context) {
	if spaOpenErr != nil {
		spaUnavailable(c)
		return
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(spaFS, path); err != nil {
		if isAssetPath(path) {
			c.Status(http.StatusNotFound)
			return
		}
		path = "index.html"
	}
	// index.html must be written directly: serving it through FileFromFS
	// triggers the stdlib's "strip index.html" local redirect (301).
	if path == "index.html" {
		data, err := fs.ReadFile(spaFS, "index.html")
		if err != nil {
			spaUnavailable(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, indexMIME, data)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.FileFromFS(path, http.FS(spaFS))
}

// isAssetPath reports whether the path looks like a static asset rather
// than a client-side route (which are extension-free by convention).
func isAssetPath(path string) bool {
	base := path
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
