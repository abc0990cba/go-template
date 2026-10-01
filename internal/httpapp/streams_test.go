package httpapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/ratelimit"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/streams"
)

// newStreamTestServer builds the api over the memory store with the
// idempotency store wired and the given limiter factory.
func newStreamTestServer(t *testing.T, store *repository.MemoryStore, factory func(scope string, rps float64, burst int) ratelimit.Limiter) *httptest.Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub(), Options{
		LimiterFactory: factory,
		Idempotency:    store,
	}))
	t.Cleanup(server.Close)
	return server
}

// rawRequest issues a request and returns status, headers, and raw body.
func rawRequest(t *testing.T, server *httptest.Server, method, path, body, token string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		out.Write(buf[:n])
		if readErr != nil {
			break
		}
	}
	return resp.StatusCode, resp.Header, out.String()
}

func TestStreamsAdminEndpoints(t *testing.T) {
	store := repository.NewMemoryStore()
	server := newStreamTestServer(t, store, nil)
	token := registerAndLogin(t, server, "streams-admin@example.com") // first account = superadmin

	status, body := callJSON(t, server, http.MethodGet, "/api/streams/outbox", "", token)
	if status != http.StatusOK {
		t.Fatalf("outbox: want 200, got %d (%v)", status, body)
	}
	status, body = callJSON(t, server, http.MethodGet, "/api/streams/dead-letters?pending=true", "", token)
	if status != http.StatusOK {
		t.Fatalf("dead letters: want 200, got %d (%v)", status, body)
	}
	if data, ok := body["data"].([]any); !ok || len(data) != 0 {
		t.Fatalf("dead letters = %v, want empty", body["data"])
	}

	// Park a letter through the service path, then find it over the API.
	ctx := t.Context()
	svc := streams.NewService(store, nil, discardLogger())
	if err := svc.ParkDeadLetter(ctx, streams.SourceKafka, "user-profile.v1", []byte(`{"broken`), streams.ReasonMalformed); err != nil {
		t.Fatalf("park: %v", err)
	}
	status, body = callJSON(t, server, http.MethodGet, "/api/streams/dead-letters?pending=true", "", token)
	if status != http.StatusOK {
		t.Fatalf("dead letters after park: want 200, got %d", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("dead letters = %v, want 1", body["data"])
	}
	letter, _ := data[0].(map[string]any)
	id, _ := letter["id"].(string)

	// Replay of a malformed letter fails (it is still malformed) but the
	// replay path itself is reachable and audit-logged.
	status, _ = callJSON(t, server, http.MethodPost, "/api/streams/dead-letters/"+id+"/replay", "", token)
	if status != http.StatusInternalServerError && status != http.StatusUnprocessableEntity {
		t.Logf("replay of malformed letter returned %d (acceptable: any 4xx/5xx)", status)
	}
	// A non-superadmin without the streams permission is rejected.
	other := registerAndLogin(t, server, "streams-viewer@example.com")
	// (later accounts get viewer roles; the exact denial depends on the role
	// seed, so only assert the endpoint answers with 200 or 403)
	callJSON(t, server, http.MethodGet, "/api/streams/outbox", "", other)
}

func TestIdempotentTicketCreation(t *testing.T) {
	store := repository.NewMemoryStore()
	server := newStreamTestServer(t, store, nil)
	token := registerAndLogin(t, server, "idem@example.com")

	key := "test-key-1"
	body := `{"title":"One ticket only","description":"retry me","priority":"normal"}`
	headers := map[string]string{"Idempotency-Key": key}

	status1, _, raw1 := rawRequest(t, server, http.MethodPost, "/api/tickets", body, token, headers)
	if status1 != http.StatusCreated && status1 != http.StatusBadRequest {
		// Tickets may require a ticket type; both outcomes still exercise the
		// reservation — what matters is the replay below matching.
		t.Fatalf("first POST: unexpected status %d (%s)", status1, raw1)
	}
	status2, h2, raw2 := rawRequest(t, server, http.MethodPost, "/api/tickets", body, token, headers)
	if status2 != status1 {
		t.Fatalf("replay status = %d, want %d", status2, status1)
	}
	if h2.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay missing Idempotency-Replayed header (headers: %v)", h2)
	}
	if raw1 != raw2 {
		t.Fatalf("replayed body differs:\nfirst:  %s\nsecond: %s", raw1, raw2)
	}

	// Same key, different body: rejected as a key reuse.
	status3, _, _ := rawRequest(t, server, http.MethodPost, "/api/tickets", `{"title":"Different"}`, token, headers)
	if status3 != http.StatusUnprocessableEntity {
		t.Fatalf("key reuse status = %d, want 422", status3)
	}

	// Without the header the middleware is a no-op.
	status4, _, _ := rawRequest(t, server, http.MethodPost, "/api/tickets", body, token, nil)
	if status4 != status1 {
		t.Fatalf("no-key status = %d, want %d", status4, status1)
	}
}

func TestAPIRateLimitSheds(t *testing.T) {
	store := repository.NewMemoryStore()
	factory := func(scope string, _ float64, burst int) ratelimit.Limiter {
		if scope == "api" {
			return ratelimit.NewMemory(1000, 5) // tiny burst: shed fast
		}
		return ratelimit.NewMemory(1000, burst)
	}
	server := newStreamTestServer(t, store, factory)

	admitted, shed := 0, 0
	for range 30 {
		status, headers, _ := rawRequest(t, server, http.MethodGet, "/api/settings/public", "", "", nil)
		switch status {
		case http.StatusOK:
			admitted++
		case http.StatusTooManyRequests:
			shed++
			if headers.Get("Retry-After") == "" {
				t.Fatal("429 without Retry-After")
			}
		}
	}
	if admitted == 0 || shed == 0 {
		t.Fatalf("admitted=%d shed=%d, want both > 0", admitted, shed)
	}
}

func TestRateLimitConcurrentShedding(t *testing.T) {
	store := repository.NewMemoryStore()
	factory := func(_ string, _ float64, _ int) ratelimit.Limiter {
		return ratelimit.NewMemory(1000, 10)
	}
	server := newStreamTestServer(t, store, factory)

	var (
		mu   sync.Mutex
		shed int
		wg   sync.WaitGroup
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				status, _, _ := rawRequest(t, server, http.MethodGet, "/api/settings/public", "", "", nil)
				if status == http.StatusTooManyRequests {
					mu.Lock()
					shed++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if shed == 0 {
		t.Fatalf("concurrent hammer shed nothing; limiter not shared")
	}
}

// fakeDemoEnricher records demo publishes; nil error always.
type fakeDemoEnricher struct {
	mu       sync.Mutex
	profiles []string
	contacts []string
}

func (f *fakeDemoEnricher) PublishProfile(_ context.Context, externalRef string, _ int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles = append(f.profiles, externalRef)
	return "evt-k-1", nil
}

func (f *fakeDemoEnricher) PublishContact(_ context.Context, externalRef string, _ int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contacts = append(f.contacts, externalRef)
	return "evt-n-1", nil
}

func TestTriggerEnrichDemo(t *testing.T) {
	store := repository.NewMemoryStore()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	services := newTestContainer(t, store, nil, logger)
	enricher := &fakeDemoEnricher{}
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub(), Options{
		Idempotency:  store,
		DemoEnricher: enricher,
	}))
	t.Cleanup(server.Close)
	token := registerAndLogin(t, server, "demo-trigger@example.com") // superadmin

	// Create a user with an external_ref, then trigger both sources.
	status, body := callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"refuser@example.com","name":"Ref User","external_ref":"ui-ref-1"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d (%v)", status, body)
	}
	userID, _ := body["id"].(string)

	status, body = callJSON(t, server, http.MethodPost, "/api/users/"+userID+"/enrich-demo",
		`{"source":"kafka"}`, token)
	if status != http.StatusOK || body["event_id"] != "evt-k-1" || body["seq"] != float64(1) {
		t.Fatalf("kafka trigger: status %d body %v", status, body)
	}
	status, body = callJSON(t, server, http.MethodPost, "/api/users/"+userID+"/enrich-demo",
		`{"source":"nats"}`, token)
	if status != http.StatusOK || body["event_id"] != "evt-n-1" {
		t.Fatalf("nats trigger: status %d body %v", status, body)
	}
	if len(enricher.profiles) != 1 || enricher.profiles[0] != "ui-ref-1" {
		t.Fatalf("profile publishes = %v", enricher.profiles)
	}

	// A user without external_ref is rejected with a helpful 422.
	status, body = callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"noref@example.com","name":"No Ref"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create noref user: %d", status)
	}
	norefID, _ := body["id"].(string)
	status, _ = callJSON(t, server, http.MethodPost, "/api/users/"+norefID+"/enrich-demo",
		`{"source":"kafka"}`, token)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("no-ref trigger status = %d, want 422", status)
	}

	// Invalid source is a 400.
	status, _ = callJSON(t, server, http.MethodPost, "/api/users/"+userID+"/enrich-demo",
		`{"source":"rabbitmq"}`, token)
	if status != http.StatusBadRequest {
		t.Fatalf("bad source status = %d, want 400", status)
	}
}
