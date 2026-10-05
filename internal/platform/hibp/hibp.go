// Package hibp checks passwords against the Have I Been Pwned corpus via
// the k-anonymity range API: only a SHA-1 prefix of the password ever
// leaves the process, never the password (or its full hash).
package hibp

import (
	"context"
	"crypto/sha1" // #nosec G505 -- SHA-1 is the HIBP range API's protocol, not a security primitive here
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultURL is the production range API.
const DefaultURL = "https://api.pwnedpasswords.com"

// cacheTTL bounds how long a negative (safe) answer is remembered; a
// compromised answer is never cached — the caller rejects immediately.
const cacheTTL = time.Hour

// cacheMax keeps the bounded answer map from growing with distinct traffic.
const cacheMax = 4096

// Client answers password-compromise questions against a Pwned Passwords
// range API. Safe for concurrent use.
type Client struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu      sync.Mutex
	safeAt  map[string]time.Time
	lookups func(ok bool) // optional request counter hook (tests)
}

// New builds a client against the given range API base URL (empty picks
// DefaultURL). The HTTP client is optional; nil uses an OTel-instrumented
// default with a hard timeout — breach checking must never stall a
// registration.
func New(baseURL string, client *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	if client == nil {
		client = &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   2 * time.Second,
		}
	}
	return &Client{
		url:    strings.TrimRight(baseURL, "/"),
		client: client,
		now:    time.Now,
		safeAt: map[string]time.Time{},
	}
}

// WithClock replaces the wall clock (tests walk cache expiry).
func (c *Client) WithClock(now func() time.Time) *Client { c.now = now; return c }

// WithObserver hooks the request counter (true = served from cache).
func (c *Client) WithObserver(lookups func(cacheHit bool)) *Client { c.lookups = lookups; return c }

// Compromised reports whether the password appears in the breach corpus.
// Network or protocol problems return an error — callers decide whether to
// fail open or closed.
func (c *Client) Compromised(ctx context.Context, password string) (bool, error) {
	digest := sha1.Sum([]byte(password)) // #nosec G401 -- the range API's protocol
	full := hex.EncodeToString(digest[:])
	prefix, suffix := full[:5], full[5:]

	if c.safeAnswered(prefix) {
		c.observe(true)
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/range/"+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("hibp request: %w", err)
	}
	req.Header.Set("Add-Padding", "true") // constant-ish response size for prefix probes
	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("hibp range %s: %w", prefix, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp range %s: status %d", prefix, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, fmt.Errorf("hibp range %s: %w", prefix, err)
	}

	compromised := false
	for _, line := range strings.Split(string(body), "\n") {
		entry, count, found := strings.Cut(strings.TrimSpace(line), ":")
		// The production corpus answers uppercase; match either case.
		if !found || !strings.EqualFold(entry, suffix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if err != nil || n > 0 {
			compromised = true
		}
		break
	}
	if !compromised {
		c.rememberSafe(prefix)
	}
	c.observe(false)
	return compromised, nil
}

// safeAnswered reports a live cached negative answer.
func (c *Client) safeAnswered(prefix string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.safeAt[prefix]
	return ok && c.now().Sub(at) < cacheTTL
}

// rememberSafe records a negative answer, evicting an arbitrary entry when
// the map is full (the cache is an optimization, not a correctness asset).
func (c *Client) rememberSafe(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.safeAt) >= cacheMax {
		for key := range c.safeAt {
			delete(c.safeAt, key)
			break
		}
	}
	c.safeAt[prefix] = c.now()
}

func (c *Client) observe(cacheHit bool) {
	if c.lookups != nil {
		c.lookups(cacheHit)
	}
}
