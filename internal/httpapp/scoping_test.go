package httpapp

// Wire-level regression for the cross-tenant fix: a plain user's SSE stream
// must not carry another organization's events, while a platform admin's
// stream does.

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/realtime"
)

// sseStream opens the event stream and returns a channel of `data:` lines.
func openScopedSSE(t *testing.T, server *httptest.Server, token string) (chan string, func()) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/events/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	lines := make(chan string, 16)
	done := make(chan struct{})
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				select {
				case lines <- line:
				case <-done:
					return
				}
			}
		}
	}()
	stop := func() {
		close(done)
		_ = resp.Body.Close()
	}
	return lines, stop
}

// waitForEvent reads events until eventType arrives or the budget ends.
func waitForEvent(t *testing.T, lines chan string, eventType string, budget time.Duration) bool {
	t.Helper()
	deadline := time.After(budget)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return false
			}
			if strings.Contains(line, `"`+eventType+`"`) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func TestSSEStreamIsOrgScoped(t *testing.T) {
	server, hub := newCfgServer(t, nil)
	admin := registerAndLogin(t, server, "scope-admin@example.com") // first user: superadmin
	other := registerAndLogin(t, server, "scope-other@example.com") // plain user, no orgs

	adminLines, stopAdmin := openScopedSSE(t, server, admin)
	defer stopAdmin()
	otherLines, stopOther := openScopedSSE(t, server, other)
	defer stopOther()

	// Both streams greet with the connected event (unrouted).
	if !waitForEvent(t, adminLines, realtime.TypeConnected, 3*time.Second) {
		t.Fatal("admin stream: no connected event")
	}
	if !waitForEvent(t, otherLines, realtime.TypeConnected, 3*time.Second) {
		t.Fatal("other stream: no connected event")
	}

	// A foreign org's ticket update: the plain user (no memberships, not an
	// admin) must never see it; the admin passes.
	hub.Publish(realtime.NewOrgEvent(realtime.TypeTicketUpdated,
		map[string]string{"title": "other org's ticket"}, "org-somewhere-else"))
	if waitForEvent(t, otherLines, realtime.TypeTicketUpdated, 700*time.Millisecond) {
		t.Fatal("cross-tenant leak: plain user received a foreign org's ticket event")
	}
	if !waitForEvent(t, adminLines, realtime.TypeTicketUpdated, 3*time.Second) {
		t.Fatal("admin must still receive org events")
	}

	// Personal notifications reach exactly their user (routed by user id,
	// not by token).
	_, me := callJSON(t, server, http.MethodGet, "/api/me", "", admin)
	adminID, _ := me["id"].(string)
	if adminID == "" {
		t.Fatal("/api/me: want non-empty id")
	}
	hub.Publish(realtime.NewUserEvent(realtime.TypeNotification,
		map[string]string{"title": "just for admin"}, adminID))
	if !waitForEvent(t, adminLines, realtime.TypeNotification, 3*time.Second) {
		t.Fatal("target user must receive their notification")
	}
	if waitForEvent(t, otherLines, realtime.TypeNotification, 500*time.Millisecond) {
		t.Fatal("personal notification leaked to another user")
	}
}
