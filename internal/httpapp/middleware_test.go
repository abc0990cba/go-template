package httpapp

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestApp() *App {
	return &App{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func serve(middleware gin.HandlerFunc, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware)
	r.GET("/", handler)
	return r
}

func TestRequestIDIsGeneratedAndUnique(t *testing.T) {
	r := serve(newTestApp().requestID(), func(c *gin.Context) { c.Status(http.StatusOK) })

	ids := make(map[string]bool, 2)
	for range 2 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		id := w.Header().Get("X-Request-ID")
		if id == "" {
			t.Fatal("response is missing X-Request-ID")
		}
		if ids[id] {
			t.Fatalf("request id %q was generated twice", id)
		}
		ids[id] = true
	}
}

func TestRequestIDHonorsIncomingHeader(t *testing.T) {
	var seen string
	r := serve(newTestApp().requestID(), func(c *gin.Context) {
		seen = c.GetString("request_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "upstream-42")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") != "upstream-42" {
		t.Errorf("response id: want upstream-42, got %q", w.Header().Get("X-Request-ID"))
	}
	if seen != "upstream-42" {
		t.Errorf("context id: want upstream-42, got %q", seen)
	}
}

func TestBodyLimitRejectsOversizedRequests(t *testing.T) {
	r := serve(newTestApp().bodyLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	tooBig := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(make([]byte, maxBodyBytes+1)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, tooBig)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: want %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
	if !strings.Contains(w.Body.String(), "request body too large") {
		t.Errorf("oversized body response: want problem title, got %q", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("empty body: want %d, got %d", http.StatusOK, w.Code)
	}
}
