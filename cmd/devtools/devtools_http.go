package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/your-org/go-template/internal/reliability"
)

// --- api client ---------------------------------------------------------------

type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func newAPIClient(base, email, password string) (*apiClient, error) {
	c := &apiClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 10 * time.Second}}
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(context.Background(), http.MethodPost, "/api/auth/login",
		map[string]string{"email": email, "password": password}, &out); err != nil {
		return nil, fmt.Errorf("login as %s: %w", email, err)
	}
	c.token = out.Token
	return c, nil
}

func (c *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// findUserByExternalRef scans the users list for the given correlation id.
func (c *apiClient) findUserByExternalRef(ctx context.Context, ref string) (map[string]any, error) {
	for offset := 0; offset < 1000; offset += 100 {
		var page struct {
			Data []map[string]any `json:"data"`
		}
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/users?limit=100&offset=%d", offset), nil, &page); err != nil {
			return nil, err
		}
		for _, user := range page.Data {
			if user["external_ref"] == ref {
				return user, nil
			}
		}
		if len(page.Data) < 100 {
			break
		}
	}
	return nil, fmt.Errorf("no user with external_ref %q found in the API", ref)
}

// --- flaky-server + demo-breaker ---------------------------------------------------------

// flakyServer answers 500 for the first failures requests, then 200.
type flakyServer struct {
	failures int64
	count    atomic.Int64
}

func (f *flakyServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if f.count.Add(1) <= f.failures {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream having a bad day"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"results":[{"name":"Recovered User"}]}`))
}

func cmdFlakyServer(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("flaky-server", flag.ExitOnError)
	var (
		addr     = fs.String("addr", ":8099", "listen address")
		failures = fs.Int64("failures", 4, "how many requests to fail before recovering")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: &flakyServer{failures: *failures}, ReadHeaderTimeout: 3 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Info("flaky server listening", "addr", *addr, "failing_first", *failures,
		"use_as", "APP_ENRICHMENT_URL=http://localhost"+*addr+"/api/?results=1")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// cmdDemoBreaker demonstrates the circuit breaker state machine in-process,
// using the exact same reliability package the backend wraps its outbound
// calls with. Run the real-server variant by pointing APP_ENRICHMENT_URL at
// `devtools flaky-server` and triggering an enrichment job.
func cmdDemoBreaker(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("demo-breaker", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Same defaults as the backend: 3 consecutive failures trip, 2s probe.
	flaky := &flakyServer{failures: 4}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: flaky, ReadHeaderTimeout: 3 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer func() { _ = srv.Close() }()

	breakers := reliability.NewBreakers(3, 2*time.Second)
	events := make(chan string, 32)
	breakers.SetOnChange(func(name string, from, to reliability.CircuitState) {
		events <- fmt.Sprintf("breaker %s: %s -> %s", name, from, to)
	})
	fmt.Printf("Firing guarded requests at a flaky upstream (%s)...\n", listener.Addr())
	client := &http.Client{Timeout: 2 * time.Second}
	for i := range 10 {
		var ( //nolint:revive // two returns from one guarded call
			status int
		)
		err := breakers.Do(context.Background(), "enrichment.http", func() error {
			resp, err := client.Get(fmt.Sprintf("http://%s/profile", listener.Addr()))
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			status = resp.StatusCode
			if resp.StatusCode >= 500 {
				return fmt.Errorf("upstream status %d", resp.StatusCode)
			}
			return nil
		})
		log.Info("request", "n", i+1, "status", status, "err", err)
		time.Sleep(400 * time.Millisecond)
	}
	time.Sleep(2200 * time.Millisecond) // let the recovery probe land
	close(events)
	var seen []string
	for event := range events {
		seen = append(seen, event)
	}
	sort.Strings(seen)
	fmt.Println("Breaker transitions:")
	for _, event := range seen {
		fmt.Println("  " + event)
	}
	fmt.Println("The backend does the same around its outbound calls — watch app_cb_state{name=\"enrichment.http\"} on /metrics.")
	return nil
}

// --- hammer (rate limiting) -----------------------------------------------------------------

func cmdHammer(args []string, _ *slog.Logger) error {
	fs := flag.NewFlagSet("hammer", flag.ExitOnError)
	var (
		target      = fs.String("target", envOr("API_BASE", "http://localhost:8080")+"/api/settings/public", "url to hammer")
		requests    = fs.Int("requests", 200, "total requests")
		concurrency = fs.Int("concurrency", 10, "parallel workers")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var (
		mu       sync.Mutex
		statuses = map[int]int{}
		wg       sync.WaitGroup
		retry    atomic.Value
	)
	work := make(chan int)
	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			for range work {
				resp, err := client.Get(*target)
				if err != nil {
					continue
				}
				if resp.StatusCode == http.StatusTooManyRequests {
					retry.Store(resp.Header.Get("Retry-After"))
				}
				_ = resp.Body.Close()
				mu.Lock()
				statuses[resp.StatusCode]++
				mu.Unlock()
			}
		}()
	}
	for i := range *requests {
		work <- i
	}
	close(work)
	wg.Wait()

	codes := make([]int, 0, len(statuses))
	for code := range statuses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	fmt.Printf("Hammered %s with %d requests:\n", *target, *requests)
	for _, code := range codes {
		fmt.Printf("  %d responses: %d\n", code, statuses[code])
	}
	if retryAfter, ok := retry.Load().(string); ok && retryAfter != "" {
		fmt.Printf("429s carried Retry-After: %ss\n", retryAfter)
	}
	if statuses[http.StatusTooManyRequests] == 0 {
		fmt.Println("No 429s observed — raise the rate or lower APP_API_RATE_RPS on the api.")
	} else {
		fmt.Println("Rate limiting is shedding excess traffic; app_rate_limit_rejected_total{scope=\"api\"} counts it on /metrics.")
	}
	return nil
}
