package httpapp

// Tests for the resilience gates added to the /api chain: the runtime
// OpenAPI validator, the load shedder, and the request timeout.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

// newCfgServer builds a router over an explicit config, for tests that need
// to flip the new gates without touching the environment. It also returns
// the realtime hub so tests can publish events into open streams.
func newCfgServer(t *testing.T, mutate func(*config.Config)) (*httptest.Server, *realtime.Hub) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if mutate != nil {
		mutate(&cfg)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	hub := realtime.NewHub()
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, hub))
	t.Cleanup(server.Close)
	return server, hub
}

func TestOpenAPIValidationRejectsOffContractBody(t *testing.T) {
	// newCfgServer wraps no contract-test gate, so the runtime middleware
	// under test is the only validator in the chain.
	server, _ := newCfgServer(t, nil)

	status, _, raw := rawRequest(t, server, http.MethodPost, "/api/auth/register", `{}`, "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("off-contract register: want 400, got %d (%s)", status, raw)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("runtime gate did not answer problem+json: %q", raw)
	}
	if title, _ := body["title"].(string); title != "request does not match the API contract" {
		t.Fatalf("want contract title, got %v", body)
	}
	if detail, _ := body["detail"].(string); detail == "" {
		t.Fatal("want a pointer at the failing field in detail")
	}
}

func TestOpenAPIValidationAcceptsOnContractBody(t *testing.T) {
	// The same flow the register helper uses must still pass the runtime
	// gate: the validator and the contract tests read one spec.
	server, _ := newCfgServer(t, nil)
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"oncontract@example.com","name":"Ada","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("on-contract register: want 201, got %d (%v)", status, body)
	}
}

func TestOpenAPIValidationLeavesUnknownRoutesToNoRoute(t *testing.T) {
	server, _ := newCfgServer(t, nil)
	status, body := callJSON(t, server, http.MethodGet, "/api/definitely-not-a-route", "", "")
	if status != http.StatusNotFound {
		t.Fatalf("unknown route: want 404, got %d (%v)", status, body)
	}
	if title, _ := body["title"].(string); title != "not found" {
		t.Fatalf("want NoRoute problem, got %v", body)
	}
}

func TestOpenAPIValidationOffModePassesBodiesThrough(t *testing.T) {
	server, _ := newCfgServer(t, func(cfg *config.Config) { cfg.OpenAPIValidation = "off" })
	// A body the contract rejects reaches the handler, which answers its own
	// validation error instead of the contract gate's.
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/register", `{}`, "")
	if status != http.StatusBadRequest {
		t.Fatalf("off-mode register: want handler 400, got %d (%v)", status, body)
	}
	if title, _ := body["title"].(string); title == "request does not match the API contract" {
		t.Fatalf("contract gate answered while off: %v", body)
	}
}

func TestOpenAPIValidationServesEmbeddedSpec(t *testing.T) {
	server := newTestServer(t)
	resp, err := server.Client().Get(server.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("GET /openapi.yaml: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi: want 200, got %d", resp.StatusCode)
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "openapi:") {
		t.Fatalf("openapi.yaml body does not look like the contract: %q", string(buf[:n]))
	}
}

func TestLoadShedShedsWhenSaturated(t *testing.T) {
	app := &App{logger: discardLogger(), shed: newLoadShed(1)}
	entered := make(chan struct{})
	release := make(chan struct{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(app.loadShedMiddleware())
	r.GET("/api/users", func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusOK)
	})

	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
		done <- w.Code
	}()
	<-entered

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("saturated server: want 503, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("503 without Retry-After")
	}
	var problem map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("503 body is not JSON: %v", err)
	}
	if problem["type"] != "about:blank" {
		t.Fatalf("503 body is not problem+json: %v", problem)
	}

	close(release)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("in-flight request: want 200 after release, got %d", code)
	}
}

func TestLoadShedLowLaneReservesHeadroom(t *testing.T) {
	shed := newLoadShed(4)
	// Three bulk requests fill the low lane (3/4 of capacity).
	for range 3 {
		if !shed.admit(laneLow) {
			t.Fatal("low lane should admit up to its reserve")
		}
	}
	if shed.admit(laneLow) {
		t.Fatal("low lane admitted past its reserve")
	}
	// The freed interactive slot is still there for normal traffic.
	if !shed.admit(laneNormal) {
		t.Fatal("normal lane should admit while under the total cap")
	}
	shed.release()
	shed.release()
	shed.release()
	shed.release()
	if !shed.admit(laneLow) {
		t.Fatal("lane should reopen once everything released")
	}
}

func TestLoadShedDisabledWhenCapZero(t *testing.T) {
	if newLoadShed(0) != nil {
		t.Fatal("cap 0 must disable load shedding")
	}
	app := &App{logger: discardLogger()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(app.loadShedMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("disabled shedder changed the response: %d", w.Code)
	}
}

func TestRequestTimeoutCancelsHandlerContext(t *testing.T) {
	app := &App{logger: discardLogger(), cfg: config.Config{RequestTimeout: 20 * time.Millisecond}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(app.requestTimeout())
	r.GET("/api/users", func(c *gin.Context) {
		select {
		case <-c.Request.Context().Done():
			httpx.Problem(c, http.StatusGatewayTimeout, "deadline exceeded", context.DeadlineExceeded)
		case <-time.After(5 * time.Second):
			c.Status(http.StatusOK) // would hang past the deadline
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("deadline did not reach the handler: %d", w.Code)
	}
}

func TestRequestTimeoutExemptsStreamsAndTransfers(t *testing.T) {
	app := &App{logger: discardLogger(), cfg: config.Config{RequestTimeout: 20 * time.Millisecond}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(app.requestTimeout())
	r.GET("/api/events/stream", func(c *gin.Context) {
		if _, ok := c.Request.Context().Deadline(); ok {
			c.Status(http.StatusExpectationFailed)
			return
		}
		c.Status(http.StatusOK)
	})
	r.GET("/api/files/abc/download", func(c *gin.Context) {
		if _, ok := c.Request.Context().Deadline(); ok {
			c.Status(http.StatusExpectationFailed)
			return
		}
		c.Status(http.StatusOK)
	})
	for _, path := range []string{"/api/events/stream", "/api/files/abc/download"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: exempt route got a deadline (%d)", path, w.Code)
		}
	}
}

func TestRequestTimeoutDisabledWhenZero(t *testing.T) {
	app := &App{logger: discardLogger()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(app.requestTimeout())
	r.GET("/", func(c *gin.Context) {
		if _, ok := c.Request.Context().Deadline(); ok {
			c.Status(http.StatusExpectationFailed)
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("disabled timeout still set a deadline: %d", w.Code)
	}
}

func TestHTTPErrorMapsDeadlineTo504(t *testing.T) {
	status, title := httpx.HTTPError(context.DeadlineExceeded)
	if status != http.StatusGatewayTimeout {
		t.Fatalf("deadline: want 504, got %d", status)
	}
	if title == "" {
		t.Fatal("deadline: want a title")
	}
}
