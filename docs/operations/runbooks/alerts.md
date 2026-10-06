# Alert runbooks

One entry per alert in `deploy/prometheus/rules/` — the 20 alerts in
`alerts.yml` plus the 4 SLO burn alerts in `slo.yml` are all covered below,
in rule-file order. Every firing alert is visible in the Alertmanager UI
(localhost:9093 with the compose observability profile; on the VPS
Alertmanager is loopback-only on 127.0.0.1:9093 — not one of the
`make vps-ui` tunnels, curl it over SSH instead). Routing per
`deploy/alertmanager/alertmanager.yml`: `severity="critical"` pages the
`critical-sink` receiver (group_wait 10s, repeat every 2h); warnings use the
default `local-sink` (group_wait 30s, repeat every 4h). Both receivers are
placeholders that POST to the api and 404 harmlessly — wire Slack,
PagerDuty, or email there when adopting the template. The entries annotated
`runbook_url` in the rules link back to this page.

The `curl` queries below target the compose Prometheus (:9090). On the VPS
Prometheus listens on 127.0.0.1:9091 — run the same query over SSH, e.g.
`ssh <vps> curl -s 'http://127.0.0.1:9091/api/v1/query?query=app_outbox_pending'`.

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

## ApiTargetDown
Prometheus cannot scrape the api `/metrics` endpoint (`up{job="go-template"} == 0`
for 1m) — the process is dead, wedged, or unreachable. **Check:**
`docker compose ps api` (or `make vps-status` on the VPS), then readiness,
which names the failing dependency:
`curl -s localhost:8080/health/ready`; logs via `docker compose logs api`
or `make vps-logs S=api`. **Fix:** restart the container
(`docker compose up -d --force-recreate api`). If readiness names a
dependency, fix that first — Postgres, Redis, or broker outages fail
readiness while the process itself is fine.

## WorkerDown
The worker's `/metrics` endpoint (`APP_METRICS_ADDR`, `worker:9090`) has
been unreachable for 2m. The worker owns the asynq handlers, the Kafka/NATS
consumers, the outbox relay, and the prune loops — while it is down, the
outbox backlog grows and enrichment stops. **Check:**
`docker compose ps worker` / `make vps-logs S=worker`; a metrics-listener
crash can leave the process half-alive, so grep the logs for `worker failed`
and `stream consumer exited; restarting`. **Fix:** restart the worker; if it
crash-loops, the `err` field on the last `worker failed` line is the cause.

## HTTPErrorRateHigh
Server errors exceed 5% of requests for 5 minutes — the fast-burn territory
of the availability SLO. The expr is the 5xx share of `app_http_requests_total`.
**Check:** the API overview dashboard ("5xx error ratio", "Errors: 5xx
ratio" by route) to find the route; pull the matching access-log lines
(`docker compose logs api | grep -E 'status=5[0-9][0-9]'` — each line carries
`method`, `path`, `status`, `duration_ms`, `request_id`) and open the
`request_id`'s trace in Jaeger. **Fix:** correlate with the last deploy or
dependency event; a dependency outage shows up here together with
BreakerOpen/BulkheadRejected — fix the dependency, not the symptom.

## HTTPErrorRateElevated
Same ratio as above but above 1% for 15m — a slow-burn budget leak rather
than an outage (docs/operations/slo.md has the math). **Check:** group the
expr by `route`
(`... / sum(rate(app_http_requests_total[5m])) by (route)`), inspect that
route's recent error logs. **Fix:** usually one noisy handler or a
degrading dependency; treat as a ticket unless it tips into
HTTPErrorRateHigh.

## HTTPLatencyHigh
p95 latency (`app_http_request_duration_seconds`) has been above the 500ms
k6 budget for 10 minutes under live traffic. **Check:** API overview →
"Latency p95 by route" to localize; then the usual suspects:
`app_pgpool_waiters_total` (pool wait — see PgPoolWaiters),
`app_http_requests_in_flight` (saturation; see LoadShedding),
`app_cb_state` (an open breaker turns calls into fast 503s, which shifts
latency elsewhere), and `app_bulkhead_in_flight` (a slow dependency backing
up). **Fix:** per cause — pool/DB tuning, raise `APP_LOADSHED_MAX_INFLIGHT`
with headroom, or fix the slow dependency; see docs/testing/performance.md
for what the budget covers.

## JobsFailing
More than 3 asynq jobs finished `failed` (`app_jobs_processed_total{status="failed"}`)
in 15m. Failed payloads retry with asynq's default backoff (job tasks get
`MaxRetry(5)`; malformed payloads return `SkipRetry` immediately) and land
in the archive when the budget is spent. **Check:** the worker logs — grep
`task failed`, each line carries `type`, `id`, and `err`; the admin Jobs
page (`GET /api/jobs`) shows job rows; Asynqmon (`make vps-ui` → 127.0.0.1:8082)
shows retry/archived per queue. **Fix:** read `err` — a dependency
(enrichment HTTP, broker, SMTP) failing is the common case; a payload bug
needs a code fix before re-triggering the flow.

## OutboxBacklog
`app_outbox_pending > 100` for 5m — events are committed to the outbox
table but the relay is not publishing them to the brokers. **Check:** first
whether the relay is alive at all (WorkerDown?); then the Resilience
dashboard's breaker panel — `kafka.produce` / `nats.publish` open means
broker publishes are failing; worker logs grep `outbox relay pass failed`
(line carries `published` and `err`). `docker compose ps kafka nats` / the
brokers profile (`docker-compose.brokers.yml`) for broker health. **Fix:**
restore the broker or the worker; if throughput (not an outage) is the
cause, raise `APP_OUTBOX_BATCH` (default 100) or shorten
`APP_OUTBOX_INTERVAL` (default 1s). Events that keep failing park as dead
letters after `APP_OUTBOX_MAX_ATTEMPTS` (default 10) — see
docs/guides/enrichment-streams.md for the pipeline.

## OutboxRelayErrors
The relay passes themselves are erroring
(`app_outbox_relay_total{result="error"}` increased in 10m) — the loop is
running but publishes fail. **Check:** same as OutboxBacklog: broker
status, the `kafka.produce`/`nats.publish` breakers, worker logs
(`outbox relay pass failed`, `err` field). **Fix:** restore the broker
before `APP_OUTBOX_MAX_ATTEMPTS` exhausts and events move to dead letters
with `reason=max_attempts` — that is the point of no return for automatic
delivery.

## DeadLettersPending
`app_dead_letters_pending > 0` for 30m — poison or retry-exhausted events
are parked awaiting human judgement. **Check:** `GET /api/streams/dead-letters`
(needs the `streams` read permission; the admin Streams page in the web UI
lists them) — the `reason` and `last_error` fields say why each one parked
(`max_attempts`, handler errors, malformed payloads). **Fix:** fix the root
cause, then `POST /api/streams/dead-letters/{id}/replay` per letter (it
re-runs through the live handler path and is audited); drop letters that
are genuinely poison. Rehearse the whole flow with `make demo-dlq`. The
Worker & queues dashboard tracks "Dead letters pending" and "Dead letters
replayed (1h)".

## BreakerOpen
The circuit breaker named by the `name` label has been failing its
dependency long enough to trip. Note the gauge mapping
(internal/reliability/circuitbreaker.go): `app_cb_state` is 0=closed,
1=open, 2=half-open, and the alert condition `== 2` sustained for 2m means
the open → half-open probe → back-to-open cycle keeps repeating — the
dependency is still down. Breaker names: `enrichment.http` (external
enrichment fetches), `kafka.produce`, `nats.publish` (broker publishes,
incl. the outbox relay), `webhooks.http` (outgoing webhook deliveries).
**Check:** Resilience dashboard → "Circuit breaker state" and "Breaker
transitions (15m)"; `curl -s 'http://localhost:9090/api/v1/query?query=app_cb_transitions_total'`
for the open/close flapping history. **Fix:** the dependency, not the
breaker — while open, calls fail fast with 503 (`ErrCircuitOpen`) instead
of hanging, and recovery is automatic via the half-open probe once
`APP_CB_OPEN_TIMEOUT` elapses and a probe succeeds.

## RateLimiterFailOpen
The rate limiter could not reach Redis (`app_rate_limit_errors_total`
increased in 5m), so every request is admitted **without** a budget check —
fail-open by design so a Redis blip does not take the API down. This is
critical because the protection is off, not because the app is. **Check:**
`docker compose ps redis`, `docker compose exec redis redis-cli ping`,
`make vps-logs S=redis` on the VPS; the Resilience dashboard's "Limiter
fail-open errors" panel. **Fix:** restore Redis. Afterwards, scan the api
access logs for the window it was open — abusive sources had a free ride.

## LiveProbeFailed
The blackbox-exporter cannot GET `/health/live` from outside the process
(`probe_success{job="blackbox-api-live"} == 0` for 1m). This is the
outside-in check: it goes through the front door (compose probes
`http://api:8080/health/live`; the VPS probe targets
`https://<app_domain>/health/live` through Caddy and TLS), so it catches a
broken router, ingress, or certificate even while whitebox metrics look
green. **Check:** `curl -v https://<domain>/health/live` from your machine,
then from the host; `make vps-logs S=caddy` for TLS/routing errors; the
blackbox UI on :9115 shows the probe's own view. **Fix:** restart Caddy or
the api as indicated; a cert problem is
`make vps-acme-production` / the ACME flow in ../deployment/vps/runbook.md.

## SLOAvailabilityFastBurn
The 99.9% availability error budget is burning at >14.4x on both the 5m and
1h windows (`slo:availability:burn_rate5m/1h` recording rules in slo.yml)
— at this rate the 30-day budget is gone in ~2 days. Treat as a page.
**Check:** the top 5xx route:
`topk(3, sum by (route) (rate(app_http_requests_total{status=~"5.."}[5m])) / sum by (route) (rate(app_http_requests_total[5m])))`,
then follow the HTTPErrorRateHigh entry. **Fix:** stop the bleeding before
shipping features — roll back the offending deploy
(`make vps-rollback` on the VPS) or fix the failing dependency.

## SLOAvailabilitySlowBurn
Availability burning at >6x on both the 30m and 6h windows — a sustained
leak emptying the budget in ~5 days. A ticket, not a page. **Check:** the
same by-route expr over `[6h]`; look for a route with a persistent 1–2%
error rate rather than an outage. **Fix:** fix or freeze the risky path;
deploy freezes are cheaper than budgets.

## SLOLatencyFastBurn
More than 5% of requests exceed the 500ms budget at >14.4x on the 5m and 1h
windows (`slo:latency:burn_rate5m/1h`; "good" = the `le="0.5"` histogram
bucket) — the 30-day latency budget is gone in ~2 days. **Check:**
"Latency p95 by route" on the API overview, then correlate with
`app_pgpool_waiters_total` and `app_cb_state` — the usual fast-burn causes
are pool saturation or a slow dependency; follow the HTTPLatencyHigh entry.

## SLOLatencySlowBurn
Latency budget burning at >6x on the 30m and 6h windows — ~5 days to empty.
**Check:** the same dashboards over a longer window; the classic causes
are N+1 queries, a colder cache (more DB work per request), and traffic
growth on unbounded list endpoints. **Fix:** a ticket with a profile
attached — Pyroscope (compose profile) makes the slow path obvious.

## Where things live

- Dashboards: Grafana → "Go Template" folder — **API overview** (RED, pg
  pool, runtime, WS, lockouts), **Worker & queues** (jobs, outbox, dead
  letters, enrichment, email), **Resilience** (breakers, bulkheads, load
  shed, limiter). Source: `deploy/grafana/dashboards/*.json`.
- Alert + SLO rules: `deploy/prometheus/rules/{alerts,slo}.yml`; Alertmanager
  routing: `deploy/alertmanager/alertmanager.yml`.
- Local stack: `docker compose --profile observability up` (see
  ../observability.md); VPS stack: `make vps-ui` and
  ../deployment/vps/runbook.md.
