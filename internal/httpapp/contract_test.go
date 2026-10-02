package httpapp

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	kinrouter "github.com/getkin/kin-openapi/routers/legacy"
)

// contractSpec loads api/openapi/openapi.yaml relative to this package and
// fails the test if the contract itself does not validate.
func contractSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	spec, err := loader.LoadFromFile(filepath.Join("..", "..", "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	if err := spec.Validate(loader.Context); err != nil {
		t.Fatalf("OpenAPI contract is invalid: %v", err)
	}
	// The test server listens on a random port while the spec names the dev
	// port; a relative server URL makes the router match by path alone.
	spec.Servers = []*openapi3.Server{{URL: "/"}}
	return spec
}

// contractApplies reports whether a path is contract surface. The SPA,
// health probes, metrics, and the docs UI live outside /api/ and predate
// no operation; the SSE stream is a long-lived push channel whose body no
// response-shape validator can collect.
func contractApplies(path string) bool {
	return strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/events/stream")
}

// contractMiddleware validates requests and responses against
// api/openapi/openapi.yaml, so the whole handler test suite doubles as a
// conformance test: a handler whose wire shapes drift from the contract
// fails here with a schema-level message before it can drift in production.
func contractMiddleware(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	spec := contractSpec(t)
	router, err := kinrouter.NewRouter(spec)
	if err != nil {
		t.Fatalf("build contract router: %v", err)
	}
	// Sanity probe: if the router could not match a known operation (e.g. a
	// spec servers entry pinning an absolute host), every request would
	// silently fall through to the passthrough branch and the gate would be
	// off while the tests stayed green.
	probe, err := http.NewRequest(http.MethodGet, "/api/me", nil)
	if err != nil {
		t.Fatalf("build contract probe: %v", err)
	}
	if _, _, err := router.FindRoute(probe); err != nil {
		t.Fatalf("contract router cannot match GET /api/me (check spec servers): %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !contractApplies(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// Transport hardening owns oversized requests: the gate never reads
		// a body the limiter should reject with 413 (the upload allowance is
		// larger still), and chunked bodies — which the SPA never sends —
		// are left to the limiter as well.
		if r.ContentLength > maxBodyBytes || len(r.TransferEncoding) > 0 {
			next.ServeHTTP(w, r)
			return
		}
		route, pathParams, err := router.FindRoute(r)
		if err != nil {
			// Not an operation of the contract (the API group's problem+json
			// NoRoute answer owns this case); let the real router respond.
			next.ServeHTTP(w, r)
			return
		}
		reqInput := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				// Real authentication is the router's job (and the handler
				// suites exercise it); the contract gate only checks shapes.
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			},
		}
		if err := openapi3filter.ValidateRequest(r.Context(), reqInput); err != nil {
			http.Error(w, "request does not match the OpenAPI contract: "+err.Error(), http.StatusBadRequest)
			return
		}
		rec := &contractResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if !rec.wrote {
			return
		}
		respInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: reqInput,
			Status:                 rec.status,
			Header:                 rec.Header(),
			Options:                &openapi3filter.Options{IncludeResponseStatus: true},
		}
		respInput.SetBodyBytes(rec.buf.Bytes())
		if err := openapi3filter.ValidateResponse(r.Context(), respInput); err != nil {
			http.Error(w, "response does not match the OpenAPI contract: "+err.Error(), http.StatusInternalServerError)
		}
	})
}

// contractResponseWriter forwards to the real writer while keeping a copy
// of the status and body for post-hoc contract validation.
type contractResponseWriter struct {
	http.ResponseWriter
	status    int
	buf       bytes.Buffer
	wrote     bool
	forwarded bool
}

func (w *contractResponseWriter) writeHeaderOnce(status int) {
	if w.forwarded {
		return
	}
	w.forwarded = true
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *contractResponseWriter) WriteHeader(status int) {
	w.writeHeaderOnce(status)
}

func (w *contractResponseWriter) Write(b []byte) (int, error) {
	// An implicit 200: the handler wrote before any explicit WriteHeader.
	w.writeHeaderOnce(http.StatusOK)
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *contractResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
