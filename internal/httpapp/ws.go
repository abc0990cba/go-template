package httpapp

// The WebSocket gateway: the bidirectional sibling of the SSE stream. It
// reads from the same realtime hub — one publisher, both transports — for
// clients that want native ping/pong, binary framing, or an easier time in
// non-browser environments. Browser same-origin policy does not protect
// WebSocket handshakes, so cross-site opens are refused here even though the
// cookie carried the session.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
)

// wsHeartbeat pings idle connections so proxies keep them open and dead
// peers surface within one interval; wsWriteTimeout bounds each frame, so a
// client (or proxy) that stops reading is dropped instead of buffering
// without bound.
const (
	wsHeartbeat    = 15 * time.Second
	wsWriteTimeout = 5 * time.Second
)

// wsStream upgrades the connection and relays hub events. Auth happens in
// the apiAuth middleware; the socket itself carries no credentials.
func (a *App) wsStream(c *gin.Context) {
	if !a.wsSameOrigin(c) {
		a.problem(c, http.StatusForbidden, "cross-origin websocket refused", nil)
		return
	}
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		// Origin was verified above; the acceptance never needs to sniff it.
		OriginPatterns:  []string{"*"},
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		// Accept already wrote the handshake error; nothing to add.
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	observ.WSConnections.Inc()
	defer observ.WSConnections.Dec()

	events, cancel := a.hub.Subscribe(a.realtimeSubscriber(c))
	defer cancel()

	// The read loop must run for the lifetime of the connection: it answers
	// the peer's pongs and notices their close. Clients have nothing to say
	// otherwise — the stream is the server's mouth, like SSE.
	go wsReadLoop(conn)

	a.wsWrite(c.Request.Context(), conn, realtime.NewEvent(realtime.TypeConnected, gin.H{"at": time.Now().UTC()}))

	heartbeat := time.NewTicker(wsHeartbeat)
	defer heartbeat.Stop()
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
			return
		case ev, ok := <-events:
			if !ok {
				_ = conn.Close(websocket.StatusGoingAway, "hub closed")
				return
			}
			if !a.wsWrite(ctx, conn, ev) {
				return
			}
		case <-heartbeat.C:
			pingCtx, pingCancel := context.WithTimeout(context.Background(), wsWriteTimeout)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				observ.WSDrops.WithLabelValues("unresponsive").Inc()
				return
			}
		}
	}
}

// wsSameOrigin refuses browser-initiated cross-site handshakes: cookies ride
// along on WebSocket upgrades, so an attacker page must not be able to open
// an authenticated socket. Non-browser clients send no Origin and pass.
func (a *App) wsSameOrigin(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Host == c.Request.Host
}

// wsWrite sends one event with a bounded deadline; a client that cannot keep
// up within wsWriteTimeout is dropped — the hub already drops slow
// subscribers rather than stalling producers, this keeps the socket honest
// too. The wire view strips the routing scope.
func (a *App) wsWrite(ctx context.Context, conn *websocket.Conn, ev realtime.Event) bool {
	data, err := json.Marshal(ev.Wire())
	if err != nil {
		return true // skip the one unserializable event, keep the stream
	}
	writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
	defer cancel()
	if err := conn.Write(writeCtx, websocket.MessageText, data); err != nil {
		observ.WSDrops.WithLabelValues(dropReason(err)).Inc()
		return false
	}
	observ.WSMessages.Inc()
	return true
}

// wsReadLoop drains client frames until the peer leaves; control-frame
// replies are handled by the library.
func wsReadLoop(conn *websocket.Conn) {
	for {
		if _, _, err := conn.Read(context.Background()); err != nil {
			return
		}
		// Client messages are ignored by design: this channel is
		// server-to-client. Anything a future protocol needs goes through a
		// deliberate message schema, not ad-hoc frames.
	}
}

func dropReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "slow_consumer"
	}
	return "write_error"
}
