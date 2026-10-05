package httpapp

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	kinrouters "github.com/getkin/kin-openapi/routers"
	kinlegacy "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	apispec "github.com/your-org/go-template/api/openapi"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/observ"
)

// maxValidateBuffer bounds the responses the response validator buffers,
// mirroring the etag middleware's budget; larger bodies stream through
// unvalidated rather than being held in memory.
const maxValidateBuffer = 2 << 20

// responseSkipPrefixes are long-lived streams and byte transfers the response
// validator never wraps: the SSE hub writes until the client leaves and file
// downloads stream past any buffering budget.
var responseSkipPrefixes = []string{"/api/events", "/api/files"}

// specValidator enforces the OpenAPI contract at runtime. The contract tests
// keep handlers honest in CI; this gate extends the same guarantee to
// handlers that reached production and to clients the tests never saw.
// Request failures answer 400 problem+json; response mismatches in "all"
// mode are logged — the bytes are already on the wire.
type specValidator struct {
	router kinrouters.Router
	logger *slog.Logger
	all    bool
}

// newSpecValidator builds the validator for the configured mode; nil means
// validation is off. A spec that fails to load disables the gate with an
// error log — the embedded copy is compile-time, so this only fires on a
// build broken badly enough that the contract tests would have failed first.
func newSpecValidator(logger *slog.Logger, mode string) *specValidator {
	if mode == "" || mode == "off" {
		return nil
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	spec, err := loader.LoadFromData(apispec.Spec)
	if err != nil {
		logger.Error("openapi validation disabled: contract does not load", "err", err)
		return nil
	}
	// Match by path alone: the spec pins a dev server URL no deployment
	// host shares, and Host-based matching would silently disable the gate.
	spec.Servers = []*openapi3.Server{{URL: "/"}}
	router, err := kinlegacy.NewRouter(spec)
	if err != nil {
		logger.Error("openapi validation disabled: contract router failed", "err", err)
		return nil
	}
	return &specValidator{router: router, logger: logger, all: mode == "all"}
}

// openapiValidate returns the group middleware; off mode passes everything
// through so the request chain stays identical to a disabled gate.
func (a *App) openapiValidate() gin.HandlerFunc {
	v := a.spec
	if v == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return v.middleware
}

func (v *specValidator) middleware(c *gin.Context) {
	// Transport hardening owns oversized requests (413) and chunked bodies;
	// validation never buffers a body the limiter should reject, and never
	// spools an upload it cannot afford to hold.
	if c.Request.ContentLength > maxBodyBytes || len(c.Request.TransferEncoding) > 0 {
		c.Next()
		return
	}
	route, pathParams, err := v.router.FindRoute(c.Request)
	if err != nil {
		// Not an operation of the contract; NoRoute answers problem+json 404.
		c.Next()
		return
	}
	reqInput := &openapi3filter.RequestValidationInput{
		Request:    c.Request,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			// Authentication is the router's job; the gate checks shapes.
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}
	if err := openapi3filter.ValidateRequest(c.Request.Context(), reqInput); err != nil {
		observ.HTTPValidationFailures.WithLabelValues("request").Inc()
		httpx.Problem(c, http.StatusBadRequest, "request does not match the API contract", err)
		c.Abort()
		return
	}
	if !v.all || skipsResponseValidation(c.Request.URL.Path) {
		c.Next()
		return
	}
	rec := &validationWriter{ResponseWriter: c.Writer}
	c.Writer = rec
	c.Next()
	if !rec.wrote || rec.full || rec.Status() < 200 || rec.Status() > 299 || rec.Status() == http.StatusNoContent {
		// Errors, empty bodies, and overflowed buffers carry nothing the
		// validator can add.
		return
	}
	respInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqInput,
		Status:                 rec.Status(),
		Header:                 rec.Header(),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	respInput.SetBodyBytes(rec.buf.Bytes())
	if err := openapi3filter.ValidateResponse(c.Request.Context(), respInput); err != nil {
		observ.HTTPValidationFailures.WithLabelValues("response").Inc()
		v.logger.WarnContext(c.Request.Context(), "response does not match the OpenAPI contract",
			"method", c.Request.Method, "path", c.Request.URL.Path, "status", rec.Status(), "err", err)
	}
}

func skipsResponseValidation(path string) bool {
	for _, prefix := range responseSkipPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// validationWriter mirrors the etag middleware's buffering writer: hold the
// body while it fits the budget, flush through once it does not.
type validationWriter struct {
	gin.ResponseWriter
	buf   bytes.Buffer
	full  bool
	wrote bool
}

func (w *validationWriter) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *validationWriter) Write(b []byte) (int, error) {
	w.wrote = true
	if !w.full {
		if w.buf.Len()+len(b) > maxValidateBuffer {
			w.full = true
			w.buf.Reset()
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}
