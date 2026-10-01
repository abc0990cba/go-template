// The shared HTTP plumbing every feature's handlers lean on: error→status
// mapping, problem+json shape, body binding, session extraction.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/reliability"
)

func testContext(method, path, body string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func TestHTTPErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantTitle  string
	}{
		{"not found", kernel.ErrNotFound, http.StatusNotFound, "not found"},
		{"conflict", kernel.ErrConflict, http.StatusConflict, "conflict"},
		{"circuit open", reliability.ErrCircuitOpen, http.StatusServiceUnavailable, "dependency temporarily unavailable"},
		{"invalid credentials", kernel.ErrInvalidCredentials, http.StatusUnauthorized, "invalid credentials"},
		{"forbidden", kernel.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"validation direct", kernel.Invalid("no good"), http.StatusBadRequest, "validation failed"},
		{"validation wrapped", fmt.Errorf("enqueue: %w", kernel.Invalid("count")), http.StatusBadRequest, "validation failed"},
		{"sentinel wrapped", fmt.Errorf("load: %w", kernel.ErrNotFound), http.StatusNotFound, "not found"},
		{"unknown", errors.New("boom"), http.StatusInternalServerError, "internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, title := HTTPError(tc.err)
			if status != tc.wantStatus || title != tc.wantTitle {
				t.Errorf("HTTPError: want (%d, %s), got (%d, %s)", tc.wantStatus, tc.wantTitle, status, title)
			}
		})
	}
}

func TestFailHidesInternalDetail(t *testing.T) {
	// Internal errors are logged, and their message must not reach the wire.
	c, rec := testContext(http.MethodGet, "/api/x", "", nil)
	Fail(c, nil, errors.New("sql: connection refused"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("internal: want 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("internal detail leaked: %s", rec.Body.String())
	}

	// Client-fixable errors carry their message as the problem detail.
	c, rec = testContext(http.MethodGet, "/api/x", "", nil)
	Fail(c, nil, kernel.Invalid("count must be positive"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("validation: want 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "count must be positive") {
		t.Errorf("validation detail missing: %s", rec.Body.String())
	}
}

func TestProblemShape(t *testing.T) {
	c, rec := testContext(http.MethodGet, "/api/x", "", nil)
	Problem(c, http.StatusTeapot, "short and stout", errors.New("details"))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("problem body: %v (%s)", err, rec.Body.String())
	}
	if body["type"] != "about:blank" || body["title"] != "short and stout" || body["detail"] != "details" {
		t.Errorf("problem fields: %v", body)
	}
}

func TestBind(t *testing.T) {
	c, rec := testContext(http.MethodPost, "/api/x", `{"name":"ok"}`, map[string]string{"Content-Type": "application/json"})
	var req struct {
		Name string `json:"name"`
	}
	if !Bind(c, &req) || req.Name != "ok" {
		t.Fatalf("bind valid body: want ok, got %q (%d)", req.Name, rec.Code)
	}

	c, rec = testContext(http.MethodPost, "/api/x", `not json`, nil)
	if Bind(c, &req) {
		t.Fatal("bind invalid body: want false")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bind invalid body: want 400, got %d", rec.Code)
	}
}

func TestBindAnswers413OnOversizedBody(t *testing.T) {
	// A MaxBytesReader-wrapped body must surface as 413, not 400.
	gin.SetMode(gin.TestMode)
	big := strings.Repeat("x", 4096)
	req := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(`{"name":"`+big+`"}`))
	req.Body = http.MaxBytesReader(nil, req.Body, 64)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	var dst map[string]any
	if Bind(c, &dst) {
		t.Fatal("oversized body: want false")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: want 413, got %d", rec.Code)
	}
}

func TestSessionTokenSources(t *testing.T) {
	c, _ := testContext(http.MethodGet, "/api/x", "", nil)
	c.Request.AddCookie(&http.Cookie{Name: "app_session", Value: "from-cookie"})
	if got := SessionToken(c); got != "from-cookie" {
		t.Errorf("cookie: want from-cookie, got %q", got)
	}

	// The header only matters without a cookie; the Bearer prefix is cut.
	c, _ = testContext(http.MethodGet, "/api/x", "", map[string]string{"Authorization": "Bearer hdr-token"})
	if got := SessionToken(c); got != "hdr-token" {
		t.Errorf("bearer: want hdr-token, got %q", got)
	}

	c, _ = testContext(http.MethodGet, "/api/x", "", map[string]string{"Authorization": "bare-token"})
	if got := SessionToken(c); got != "bare-token" {
		t.Errorf("bare header: want bare-token, got %q", got)
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	c, rec := testContext(http.MethodGet, "/api/x", "", nil)
	SetSession(c, "tok-1", 10*time.Minute, true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set session: want 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "app_session" || cookie.Value != "tok-1" || !cookie.HttpOnly || cookie.Path != "/" || !cookie.Secure {
		t.Errorf("session cookie flags: got %+v", cookie)
	}

	// Clearing expires the cookie immediately.
	c, rec = testContext(http.MethodGet, "/api/x", "", nil)
	ClearSession(c, true)
	cookie = rec.Result().Cookies()[0]
	if cookie.MaxAge != -1 {
		t.Errorf("clear session: want MaxAge -1, got %d", cookie.MaxAge)
	}
}

func TestContextReadersAndDefaults(t *testing.T) {
	c, _ := testContext(http.MethodGet, "/api/x", "", nil)
	if got := UserID(c); got != "" {
		t.Errorf("no user id: want empty, got %q", got)
	}
	if got := Domain(c); got != "*" {
		t.Errorf("no domain: want *, got %q", got)
	}

	c, _ = testContext(http.MethodGet, "/api/x", "", nil)
	c.Set("user_id", "u7")
	c.Set("domain", "org-42")
	if got := UserID(c); got != "u7" {
		t.Errorf("user id: want u7, got %q", got)
	}
	if got := Domain(c); got != "org-42" {
		t.Errorf("domain: want org-42, got %q", got)
	}
}

func TestIntQueryAndListOptions(t *testing.T) {
	c, _ := testContext(http.MethodGet, "/api/x?limit=7&offset=3&mode=cursor&after=abc&q=find&status=done&type=report", "", nil)
	if got := IntQuery(c, "limit", 25); got != 7 {
		t.Errorf("limit: want 7, got %d", got)
	}
	if got := IntQuery(c, "missing", 25); got != 25 {
		t.Errorf("missing query: want fallback 25, got %d", got)
	}
	if got := IntQuery(c, "limit", 25); got != 7 {
		t.Errorf("limit again: want 7, got %d", got)
	}
	opts := ListOptions(c)
	if opts.Query != "find" || opts.Mode != "cursor" || opts.Limit != 7 || opts.Offset != 3 || opts.After != "abc" || opts.Status != "done" || opts.Type != "report" {
		t.Errorf("list options: got %+v", opts)
	}
}

func TestListOrEmpty(t *testing.T) {
	var nilSlice []string
	if got := ListOrEmpty(nilSlice); got == nil {
		t.Error("nil slice: want empty slice, got nil")
	}
	filled := []int{1}
	if got := ListOrEmpty(filled); len(got) != 1 {
		t.Errorf("filled slice: want preserved, got %v", got)
	}
}

func TestClientSafeMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"validation", kernel.Invalid("bad input"), true},
		{"not found", kernel.ErrNotFound, true},
		{"conflict", kernel.ErrConflict, true},
		{"credentials", kernel.ErrInvalidCredentials, true},
		{"internal", errors.New("panic in the matrix"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ClientSafeMessage(tc.err)
			if ok != tc.want {
				t.Errorf("ClientSafeMessage: want %v, got %v", tc.want, ok)
			}
		})
	}
}
