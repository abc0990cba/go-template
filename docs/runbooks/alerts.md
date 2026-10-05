# Alert runbooks

One entry per alert in `deploy/prometheus/rules/`. Every alert is visible in
the Alertmanager UI (`make vps-ui` → port 9093) and routed per
`alert_receiver`.

## PgPoolWaiters
The pgx pool is opening connections or queuing acquires at >1/s for 5
minutes. **Check:** p95 latency by route (Grafana → API overview); a slow
query or a new traffic source usually precedes it. **Fix:** raise
`APP_PG_MAX_CONNS` (Postgres `max_connections` must stay above all pools
combined), or find the slow query. Postgres itself is fine until
`app_pgpool_canceled_acquires_total` climbs — that means requests are timing
out waiting.

## PgPoolExhausted
All pool connections are checked out. **Check:** worker/api logs for a stuck
transaction; `pg_stat_activity` for long queries. **Fix:** restart the
affected process to release slots, then fix the leak. This alert is
critical because new requests fail outright.

## LoadShedding
The shedder answered 503s. **Check:** the `lane` label — `low` means bulk
endpoints (reports/analytics/exports) are saturated, `normal` means
interactive traffic is being refused. **Fix:** raise
`APP_LOADSHED_MAX_INFLIGHT` only if CPU/DB have headroom; otherwise you are
at capacity and shedding is doing its job.

## ContractValidationFailures
Requests failing runtime OpenAPI validation (>10 in 15m). **Check:** the api
log lines around the failures name the route and field. If a legitimate
client drifts, fix the client; if the spec is wrong, fix the spec — never
disable the gate to make an error go away.

## BulkheadRejected
An outbound dependency's concurrency cap is rejecting calls. **Check:** the
`name` label (HTTP enrichment, broker publish...); the matching circuit
breaker panel shows whether it is also opening. **Fix:** raise
`APP_BULKHEAD_LIMIT` if the dependency is healthy but slow; otherwise fix
the dependency.

## WebSocketMassDrops
Server-side WS drops >10 in 10m. `slow_consumer` = a client/proxy stopped
reading frames (usually a buffering proxy in front of the app — WS needs
`X-Accel-Buffering: no`-equivalent pass-through); `unresponsive` = ping
timeouts (network or a paused client). **Check:** proxy config first, then
`app_ws_connections` for a cliff.

## EmailDeliveriesDead
Messages exhausted 5 delivery attempts. **Check:** `GET /api/emails?status=dead`
(admin) for `last_error` — auth failure, DNS, or SMTP rejection. **Fix:**
restore SMTP, then `POST /api/emails/{id}/retry` per message (or re-trigger
the underlying flow; password resets mint fresh tokens anyway).

## LockoutStorm
>5 accounts locked in 15m — almost always distributed credential stuffing
(per-IP limits cap one source; the spread defeats them). **Check:** auth
logs for failing-source IPs; block at the edge (Caddy/firewall) if a pattern
emerges. **Do not** disable lockout — it is doing its job; raise
`APP_LOCKOUT_THRESHOLD` only with evidence of false positives.
