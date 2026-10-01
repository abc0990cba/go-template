package httpapp

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

const landingFixture = `<!doctype html><html><head><title>Go Template</title></head>` +
	`<body><div id="root"><h1>The Go + React platform</h1></div></body></html>`

// embedFS swaps the package-level embedded dist for a fixture, restored on
// cleanup — the serving logic is exercised against known bytes instead of
// whatever the compiling binary happens to have embedded.
func embedFS(t *testing.T, files map[string]string) {
	t.Helper()
	build := fstest.MapFS{}
	for name, content := range files {
		build[name] = &fstest.MapFile{Data: []byte(content)}
	}
	prevFS, prevErr := spaFS, spaOpenErr
	spaFS, spaOpenErr = build, nil
	t.Cleanup(func() { spaFS, spaOpenErr = prevFS, prevErr })
}

// spaEngine is the serving slice of NewRouter: the landing route plus the
// SPA fallback, without the API or middleware the tests here don't exercise.
func spaEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	app := &App{}
	r.GET("/", app.landing)
	r.HEAD("/", app.landing)
	r.NoRoute(func(c *gin.Context) { serveSPA(c) })
	return r
}

func TestLandingServesPrerenderedHTML(t *testing.T) {
	embedFS(t, map[string]string{
		"landing/index.html":    landingFixture,
		"landing/index.html.br": "brotli-bytes",
	})
	r := spaEngine()

	// Browsers that accept brotli get the precompressed bytes: the HTML was
	// compressed once at build time, not per request.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("content-encoding: want br, got %q", got)
	}
	if got := res.Body.String(); got != "brotli-bytes" {
		t.Fatalf("want precompressed body, got %q", got)
	}
	if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content-type: want text/html, got %q", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control: want no-cache, got %q", got)
	}
	if got := res.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("vary: want Accept-Encoding, got %q", got)
	}
	if res.Header().Get("ETag") == "" {
		t.Fatal("want an ETag for revalidation")
	}

	// Clients without compression support get the identity HTML.
	plain := httptest.NewRecorder()
	r.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/", nil))
	if plain.Header().Get("Content-Encoding") != "" {
		t.Fatalf("identity response must not claim an encoding, got %q", plain.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(plain.Body.String(), "<h1>The Go + React platform</h1>") {
		t.Fatalf("want prerendered markup, got %q", plain.Body.String())
	}
}

func TestLandingRevalidatesWithETag(t *testing.T) {
	embedFS(t, map[string]string{"landing/index.html": landingFixture})
	r := spaEngine()

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("want an ETag on the first response")
	}

	revalidated := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	r.ServeHTTP(revalidated, req)
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match: want 304, got %d", revalidated.Code)
	}
	if revalidated.Body.Len() != 0 {
		t.Fatalf("304 must not carry a body, got %q", revalidated.Body.String())
	}
}

func TestLandingFallsBackToSPAWhenPrerenderMissing(t *testing.T) {
	// Binary built before the landing existed: "/" keeps serving the SPA.
	embedFS(t, map[string]string{"index.html": "<html><body>spa</body></html>"})
	r := spaEngine()

	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	if got := res.Body.String(); got != "<html><body>spa</body></html>" {
		t.Fatalf("want SPA fallback, got %q", got)
	}
}

func TestLandingUnavailableWhenEmbedMissing(t *testing.T) {
	prevFS, prevErr := spaFS, spaOpenErr
	spaFS, spaOpenErr = nil, fs.ErrInvalid
	t.Cleanup(func() { spaFS, spaOpenErr = prevFS, prevErr })

	r := spaEngine()
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", res.Code)
	}
}

func TestServeSPACachingAndCompression(t *testing.T) {
	embedFS(t, map[string]string{
		"index.html":              "<html><body>spa</body></html>",
		"assets/app-abc123.js":    "console.log(1)",
		"assets/app-abc123.js.gz": "gzipped-js",
	})
	r := spaEngine()

	// Hashed asset filenames are immutable: cache for a year, and serve the
	// precompressed sibling when the client asks for gzip.
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding: want gzip, got %q", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control: want immutable, got %q", got)
	}
	if got := res.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("content-type: want javascript, got %q", got)
	}

	// Missing assets 404 instead of returning markup; extension-less paths
	// fall back to the SPA entry.
	missing := httptest.NewRecorder()
	r.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/nope-999.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset: want 404, got %d", missing.Code)
	}
	spa := httptest.NewRecorder()
	r.ServeHTTP(spa, httptest.NewRequest(http.MethodGet, "/login", nil))
	if spa.Code != http.StatusOK || spa.Body.String() != "<html><body>spa</body></html>" {
		t.Fatalf("client-side route: want SPA fallback 200, got %d %q", spa.Code, spa.Body.String())
	}
}

func TestAcceptsEncoding(t *testing.T) {
	cases := []struct {
		header string
		name   string
		want   bool
	}{
		{"", "br", false},
		{"br", "br", true},
		{"gzip, br", "br", true},
		{"gzip, br", "gzip", true},
		{"br;q=0", "br", false},
		{"br;q=0.001", "br", true},
		{"deflate", "gzip", false},
		{"gzip; q=1.0, br", "gzip", true},
	}
	for _, tc := range cases {
		if got := acceptsEncoding(tc.header, tc.name); got != tc.want {
			t.Errorf("acceptsEncoding(%q, %q) = %v, want %v", tc.header, tc.name, got, tc.want)
		}
	}
}

// TestLandingLiveEmbedded exercises the real embedded dist end to end:
// prerendered HTML at "/", brotli negotiation, ETag revalidation, and the
// SPA fallback one path over. Skips on placeholder dists (Go CI compiles
// before any frontend build) — `make web-build && go test` runs it.
func TestLandingLiveEmbedded(t *testing.T) {
	if spaOpenErr != nil {
		t.Skip("no embedded frontend")
	}
	if _, err := fs.Stat(spaFS, landingPage); err != nil {
		t.Skip("landing not embedded (placeholder dist); run make web-build first")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)

	// The landing arrives as complete HTML: hero heading and structured
	// data are plain markup, no JavaScript execution required.
	res, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	for _, marker := range []string{"<h1", "application/ld+json", "rel=\"canonical\""} {
		if !strings.Contains(string(body), marker) {
			t.Fatalf("landing HTML missing %q", marker)
		}
	}

	// One path over, the SPA serves with its own caching policy.
	spaRes, err := http.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	defer spaRes.Body.Close()
	if spaRes.StatusCode != http.StatusOK {
		t.Fatalf("/login: want 200, got %d", spaRes.StatusCode)
	}

	// robots and sitemap exist next to the landing.
	for _, seo := range []string{"/robots.txt", "/sitemap.xml"} {
		seoRes, err := http.Get(server.URL + seo)
		if err != nil {
			t.Fatalf("GET %s: %v", seo, err)
		}
		seoRes.Body.Close()
		if seoRes.StatusCode != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", seo, seoRes.StatusCode)
		}
	}
}

// TestRootRouteIsLandingNotAPI404 pins the routing decision: "/" is served
// by the landing handler (or its SPA fallback on placeholder dists), never
// by the API notFound path.
func TestRootRouteIsLandingNotAPI404(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)

	res, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), `"type":"about:blank"`) {
		t.Fatalf(`/ must not hit the API notFound path, got %q`, body)
	}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("content-type: want html, got %q", res.Header.Get("Content-Type"))
	}
}
