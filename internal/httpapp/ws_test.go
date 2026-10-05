package httpapp

// The WebSocket gateway: a real client dials the upgraded endpoint and must
// see hub events; unauthenticated and cross-site handshakes must fail before
// the upgrade.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/your-org/go-template/internal/realtime"
)

func dialWS(t *testing.T, serverURL, token, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	opts := &websocket.DialOptions{}
	if token != "" {
		opts.HTTPHeader = http.Header{"Authorization": []string{"Bearer " + token}}
	}
	if origin != "" {
		if opts.HTTPHeader == nil {
			opts.HTTPHeader = http.Header{}
		}
		opts.HTTPHeader.Set("Origin", origin)
	}
	return websocket.Dial(ctx, "ws://"+hostOf(serverURL)+"/api/events/ws", opts)
}

func hostOf(serverURL string) string {
	// httptest servers listen on http://127.0.0.1:port; ws:// shares the
	// host:port.
	return strings.TrimPrefix(serverURL, "http://")
}

func readEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var ev map[string]any
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("frame is not JSON: %v", err)
	}
	return ev
}

func TestWebSocketRelaysHubEvents(t *testing.T) {
	server, hub := newCfgServer(t, nil)
	token := registerAndLogin(t, server, "ws-user@example.com")

	conn, _, err := dialWS(t, server.URL, token, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	connected := readEvent(t, conn)
	if connected["type"] != realtime.TypeConnected {
		t.Fatalf("first frame: want connected, got %v", connected)
	}

	hub.Publish(realtime.NewEvent(realtime.TypeJobUpdated, map[string]any{"id": "j1", "status": "done"}))
	ev := readEvent(t, conn)
	if ev["type"] != realtime.TypeJobUpdated {
		t.Fatalf("relayed frame: want job.updated, got %v", ev)
	}
}

func TestWebSocketRejectsAnonymousAndCrossSite(t *testing.T) {
	server, _ := newCfgServer(t, nil)
	token := registerAndLogin(t, server, "ws-guard@example.com")

	if conn, resp, err := dialWS(t, server.URL, "", ""); err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		t.Fatalf("anonymous dial must fail, got upgrade with status %d", resp.StatusCode)
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("anonymous dial: want 401, got %d (err %v)", status, err)
	}

	// A valid session from an attacker page: auth passes, the origin check
	// must still refuse the upgrade (cookies ride along on WS handshakes).
	if conn, resp, err := dialWS(t, server.URL, token, "https://evil.example"); err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		t.Fatalf("cross-site dial must fail, got upgrade with status %d", resp.StatusCode)
	} else if resp == nil || resp.StatusCode != http.StatusForbidden {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("cross-site dial: want 403, got %d (err %v)", status, err)
	}
}

func TestWebSocketSameOriginAccepted(t *testing.T) {
	server, _ := newCfgServer(t, nil)
	token := registerAndLogin(t, server, "ws-origin@example.com")

	// The SPA dials with an Origin matching the API host.
	conn, _, err := dialWS(t, server.URL, token, server.URL)
	if err != nil {
		t.Fatalf("same-origin dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	if ev := readEvent(t, conn); ev["type"] != realtime.TypeConnected {
		t.Fatalf("same-origin stream: unexpected first frame %v", ev)
	}
}
