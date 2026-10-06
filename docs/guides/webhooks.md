# Webhooks (outgoing)

Outgoing webhooks let external systems react to in-app events: an endpoint
subscribes to analytics event names, and every matching event is POSTed as
JSON, signed with the endpoint's secret. Code: `internal/webhooks/`
(service + delivery), `internal/webhooks/http.go` (routes), tables from
`migrations/postgres/000011_webhooks.*`. The admin UI page is
`web/src/routes/admin.webhooks.tsx`; the demo seed ships one inactive
sample endpoint with a delivery history.

## How an event reaches an endpoint

1. Something records an analytics event; the realtime fan sink forwards
   every one to the webhook service (`realtime.WebhookSink` registered on
   the `FanSink` in `cmd/api/main.go`).
2. `HandleEvent` checks the feature flag, builds a payload
   `{"event", "user_id", "occurred_at"}`, lists endpoints, and matches
   subscriptions: exact event name or `"*"` for everything; inactive
   endpoints are skipped.
3. Each match inserts a `webhook_deliveries` row (status `pending`) and
   hands the delivery to a dispatcher (below). Nothing blocks the request
   path — the delivery log carries the outcome.
4. The dispatcher POSTs the raw body and records the attempt: 2xx →
   `success` (`delivered_at` set), anything else (or a transport error) →
   `failed` with `last_response_status` / `last_error` and a bumped
   `attempts` counter.

The `webhooks.enabled` flag (admin settings, `GET/PUT /api/settings`,
default `true`) short-circuits both the fan-out and the delivery POST. A
delivery stuck in `pending` with no attempts is the signature of the flag
having been turned off.

## Dispatch: queue or inline

- **With Redis** (`APP_REDIS_ADDR` set): the api enqueues an asynq task
  `webhook.deliver` (`asynq.TaskID(delivery.ID)` dedupes producer retries,
  `MaxRetry(3)`, 30s timeout, `default` queue). The worker loads the
  endpoint + delivery by id and posts once per task execution; asynq's
  default backoff spaces the retries. Trace context rides in the task
  headers, so delivery spans join the producing request's trace.
- **Without Redis** (single-process dev mode): an inline dispatcher runs
  the delivery in a detached goroutine with up to 3 attempts (0s / 2s /
  8s), stopping early on success.

Both paths share one POST implementation, protected like every outbound
call: a bulkhead named `webhooks.http` (`APP_BULKHEAD_LIMIT`) wraps a
circuit breaker also named `webhooks.http` — a dead endpoint trips the
breaker and deliveries fail fast instead of piling up timeouts. The HTTP
client inherits `APP_EXTERNAL_TIMEOUT` (default 3s) and emits otel spans.
There are no webhook-specific metrics; watch
`app_cb_state{name="webhooks.http"}`,
`app_bulkhead_rejected_total{name="webhooks.http"}`, and the delivery log.

## Security

- Endpoint URLs must be absolute `http(s)` — rejected otherwise.
- The secret is generated server-side when omitted and returned **once**
  in the create response; the struct field is tagged `json:"-"`, so it
  never appears in any other response.
- Every delivery is signed: `X-Template-Signature: sha256=<hexdigest>` is
  HMAC-SHA256 over the exact raw body bytes keyed with the endpoint
  secret; `X-Template-Delivery` carries the delivery id for idempotent
  receiver-side dedupe.

Receiver-side verification (Go):

```go
func verify(secret string, body []byte, sig string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(sig, prefix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig[len(prefix):]))
}
```

Read the raw body before decoding, verify against it, and dedupe on seen
`X-Template-Delivery` ids if your handler is not idempotent — at-least-once
delivery is the contract.

## API surface

All routes are under `/api`, permission-gated by the `webhooks` catalog
object (the `admin` role holds all actions):

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/api/webhooks` | `webhooks:read` | List endpoints (secrets never included) |
| POST | `/api/webhooks` | `webhooks:create` | Create; 201 returns `{id, url, secret, events, active}` — the only time the secret is visible |
| PUT | `/api/webhooks/{id}` | `webhooks:update` | Rewrite url/events/description/active |
| DELETE | `/api/webhooks/{id}` | `webhooks:delete` | Delete; deliveries cascade |
| GET | `/api/webhooks/{id}/deliveries?limit=25` | `webhooks:read` | Delivery log, newest first |
| POST | `/api/webhooks/{id}/test` | `webhooks:execute` | Fire a synthetic `webhook.test` event through the normal path; 202 |

Mutations write audit records (`webhook.created` / `webhook.updated` /
`webhook.deleted`), visible on the admin audit timeline.

## Failure and retry behavior

Delivery is **at-least-once**. Queue mode: 1 initial attempt + 3 asynq
retries with default backoff; each execution posts once and updates the
delivery row, so `attempts` and `last_error` stay current. A task that
keeps failing ends in asynq's archive (visible in Asynqmon,
`make vps-ui` → 8082 on the VPS). Inline mode: 3 attempts at 0s/2s/8s,
then the delivery stays `failed`. There is no manual redeliver button —
`POST .../test` is the operator's re-check; otherwise trigger the
producing event again.

## Try it locally

Start a throwaway receiver in a second terminal (any HTTP echo works; this
one also verifies the signature — `go run ./cmd/wh-echo` style):

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-Template-Signature")
		ok := false
		if strings.HasPrefix(sig, "sha256=") {
			mac := hmac.New(sha256.New, []byte("SECRET-FROM-CREATE-RESPONSE"))
			mac.Write(body)
			ok = hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig[7:]))
		}
		log.Printf("sig ok: %v  delivery: %s  body: %s",
			ok, r.Header.Get("X-Template-Delivery"), body)
		w.WriteHeader(200)
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:9099", nil))
}
```

Then, against the compose stack (`make docker-up`):

```bash
# 1. Create an endpoint (the secret comes back exactly once)
curl -s -X POST localhost:8080/api/webhooks -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://host.docker.internal:9099/hook","events":["*"],"description":"local echo"}'

# 2. Fire a synthetic event (no real traffic needed)
curl -s -X POST localhost:8080/api/webhooks/$ID/test -H "Authorization: Bearer $TOKEN"

# 3. Watch the delivery log
curl -s localhost:8080/api/webhooks/$ID/deliveries -H "Authorization: Bearer $TOKEN"
```

The api reaches your machine via `host.docker.internal` (Docker Desktop /
OrbStack); point the URL at any tunnel instead for external receivers.

## Related

- [Enrichment streams](enrichment-streams.md) — the broker/outbox path for
  `user.enriched` events; webhooks and the outbox are independent fans out
  of the same analytics event flow.
- [Jobs](jobs.md) — the asynq queue the delivery tasks ride on.
- [Alert runbook](../operations/runbooks/alerts.md) — `BreakerOpen` for
  `name="webhooks.http"` and the bulkhead entries.
