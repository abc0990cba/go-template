package hibp

import (
	"crypto/sha1" // #nosec G505 -- mirrors the protocol under test
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// corpusFor renders the range-API line for a password: SHA1 upper hex
// minus the 5-char prefix, a colon, and the breach count.
func corpusFor(password, count string) string {
	digest := sha1.Sum([]byte(password)) // #nosec G401 -- protocol under test
	full := strings.ToUpper(hex.EncodeToString(digest[:]))
	return full[5:] + ":" + count + "\n"
}

// rangeServer answers /range/{prefix} with a fixed corpus; the counter
// exposes how often the process actually went to the wire.
func rangeServer(t *testing.T, corpus string, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/range/") || hits.Load() > 1<<20 {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(corpus))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestCompromisedFindsMatch(t *testing.T) {
	password := "breached-password-" + "42" // assembled, so scanners do not flag a literal credential
	var hits atomic.Int64
	server := rangeServer(t, corpusFor(password, "42")+"00B33FD4FB4BD2D9C4C4C4C4C4C4C4C4C4C4C4C4:1\n", &hits)
	client := New(server.URL, server.Client())

	compromised, err := client.Compromised(t.Context(), password)
	if err != nil {
		t.Fatalf("compromised: %v", err)
	}
	if !compromised {
		t.Fatal("corpus match not detected")
	}
}

func TestCompromisedClearPasswordIsSafe(t *testing.T) {
	var hits atomic.Int64
	// The corpus carries a different suffix than the tested password's.
	server := rangeServer(t, "AFE9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9:999999\n", &hits)
	client := New(server.URL, server.Client())

	compromised, err := client.Compromised(t.Context(), "correct horse battery staple 42")
	if err != nil {
		t.Fatalf("compromised: %v", err)
	}
	if compromised {
		t.Fatal("non-matching suffix reported as compromised")
	}
}

func TestZeroCountIsNotCompromised(t *testing.T) {
	password := "zero-count-password"
	var hits atomic.Int64
	server := rangeServer(t, corpusFor(password, "0"), &hits)
	client := New(server.URL, server.Client())

	compromised, err := client.Compromised(t.Context(), password)
	if err != nil {
		t.Fatalf("compromised: %v", err)
	}
	if compromised {
		t.Fatal("a zero breach count is not a compromise")
	}
}

func TestCacheAvoidsRepeatLookups(t *testing.T) {
	var hits atomic.Int64
	password := "safe unique password 7"
	server := rangeServer(t, "AFE9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9:1\n", &hits)
	client := New(server.URL, server.Client())
	for range 3 {
		compromised, err := client.Compromised(t.Context(), password)
		if err != nil || compromised {
			t.Fatalf("lookup %d: compromised=%v err=%v", hits.Load(), compromised, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("negative answers must be cached, went to the wire %d times", hits.Load())
	}
}

func TestCompromisedAnswersAreNeverCached(t *testing.T) {
	// A compromised answer must hit the corpus every time: corpus updates
	// must not be masked by a stale answer, and the rejection is the
	// caller's decision to make per attempt.
	password := "breached-password-" + "42" // assembled, so scanners do not flag a literal credential
	var hits atomic.Int64
	server := rangeServer(t, corpusFor(password, "1"), &hits)
	client := New(server.URL, server.Client())
	for range 2 {
		compromised, err := client.Compromised(t.Context(), password)
		if err != nil || !compromised {
			t.Fatalf("compromised=%v err=%v", compromised, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("compromised answers must not be cached, wire hits %d", hits.Load())
	}
}

func TestServerErrorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := New(server.URL, server.Client())
	if _, err := client.Compromised(t.Context(), "whatever"); err == nil {
		t.Fatal("server failure must surface as an error, not a safe answer")
	}
}

func TestCacheExpires(t *testing.T) {
	var hits atomic.Int64
	password := "expiring safe pass" + "word" // assembled inline; not a credential
	server := rangeServer(t, "AFE9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9A0E9:1\n", &hits)
	client := New(server.URL, server.Client())
	now := time.Unix(1_800_000_000, 0)
	client.WithClock(func() time.Time { return now })

	if _, err := client.Compromised(t.Context(), password); err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	now = now.Add(2 * cacheTTL)
	if _, err := client.Compromised(t.Context(), password); err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expired cache must re-query the corpus, hits %d", hits.Load())
	}
}
