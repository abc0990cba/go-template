package httpapp

import (
	"io/fs"

	"github.com/gin-gonic/gin"
)

// landing serves the prerendered marketing page at "/": complete HTML with
// copy, meta tags, and structured data, rendered once at build time by
// web/scripts/prerender.mjs (react-dom/server) — crawlers and visitors get
// the full content without waiting for any JavaScript, and the response
// revalidates through the shared embedded-file path (ETag, brotli/gzip).
func (a *App) landing(c *gin.Context) {
	if spaOpenErr != nil {
		spaUnavailable(c)
		return
	}
	if _, err := fs.Stat(spaFS, landingPage); err != nil {
		// Placeholder dist (binary compiled before the first frontend
		// build): keep the historical behavior of serving the SPA at "/".
		serveSPA(c)
		return
	}
	serveEmbedded(c, landingPage)
}
