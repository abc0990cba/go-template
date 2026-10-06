# Chaos Engineering Lab

Monitoring tells you the system *is* misbehaving; chaos engineering makes
sure you already know *how* it misbehaves. This lab injects real faults —
latency, timeouts — into the paths between the app and its dependencies,
so you can rehearse failure on an ordinary Tuesday instead of learning
during an incident.

The tooling is deliberately boring: [Toxiproxy](https://github.com/Shopify/toxiproxy)
sits between the app and Postgres/Redis, and a handful of scripts toggle
faults through its HTTP API. No kernel modules, no privileged containers,
no separate cluster.

## Running the lab

```sh
# 1. The full stack with the observability profile + chaos proxies.
#    (Base services now talk to postgres/redis *through* Toxiproxy.)
docker compose -f docker-compose.yml -f docker-compose.brokers.yml \
  -f docker-compose.chaos.yml --profile chaos --profile observability up -d --build

# 2. Fault injection (TOXIPROXY_API defaults to http://localhost:8474).
make chaos-status
make chaos-pg-latency MS=2000   # +2s latency on every Postgres round-trip
make chaos-pg-down              # Postgres requests time out
make chaos-redis-down           # Redis requests time out
make chaos-restore              # remove every toxic; the app self-heals
```

## Game-day scenarios

Each experiment states a hypothesis. If the observation differs, you
found a bug — write the regression test before removing the toxic.

### 1. Postgres latency — the pool is the fuse

**Inject:** `make chaos-pg-latency MS=2000`
**Hypothesis:** requests slow down but nothing falls over; the pg pool
collector shows saturation (`app_pgpool_waiters_total` climbs, acquired ≈
max); the latency SLO burns fast and `SLOLatencyFastBurn` fires.
**Watch:** API overview dashboard → p95 latency, pg pool panels; Prometheus
alerts page.
**Restore:** `make chaos-restore` → latency recovers within seconds, burn
alerts resolve. The breaker does **not** trip — timeouts are bounded, not
failures, and the dependency is technically answering.

### 2. Postgres outage — degrade loudly, die never

**Inject:** `make chaos-pg-down`
**Hypothesis:** `/health/ready` flips to 503 naming the store (Postgres is
deliberately fatal-once-configured — ADR 0004 — so the process exits by
design); in Kubernetes the readiness gate removes the pod from rotation
instead. `ApiTargetDown` fires after the scrape misses.
**Watch:** readiness response body, Alertmanager UI, container restarts.
**Restore:** `make chaos-restore` → restart loop exits, migrations confirm
current, traffic resumes.

### 3. Redis outage — the graceful-degradation ladder

**Inject:** `make chaos-redis-down`
**Hypothesis:** this is the richest scenario because Redis backs four
features with *different* degradation contracts (ADR 0004):

| Feature | Contract during outage |
|---|---|
| Rate limiting | **fail-open** — requests admitted, `RateLimiterFailOpen` goes critical |
| Read cache | bypassed — reads hit the store, latency rises modestly |
| asynq jobs | enqueue returns 400/503 per the data-export contract; no fake success |
| SSE relay | in-process events still flow; cross-process relay pauses |

**Watch:** which of those four actually happens (this is the test).
**Restore:** budgets re-form, jobs enqueue again, no restart needed.

### 4. Webhook endpoint gone — breaker choreography

**Inject:** create a webhook pointing at a non-routable address, then
trigger events (`make demo-enrichment`).
**Hypothesis:** deliveries fail 5 times → `webhooks.http` breaker opens
(`app_cb_state{name="webhooks.http"} == 2`) → `BreakerOpen` alert fires →
after 30 s a half-open probe runs → recovery closes it. Bulkhead stays
mostly idle because the breaker rejects before slots fill.
**Watch:** Resilience dashboard, then the trace for one delivery in Jaeger:
client span → breaker annotation → failure.

### 5. Slow webhook endpoint — bulkhead vs breaker

**Inject:** point a webhook at `make flaky-server` (devtools: accepts and
sleeps), trigger events, then raise latency instead of errors.
**Hypothesis:** breaker stays closed (no failures — just slow), the
bulkhead fills (`app_bulkhead_in_flight` ≈ `APP_BULKHEAD_LIMIT`) and
excess deliveries reject fast with `ErrBulkheadFull`
(`app_bulkhead_rejected_total` climbs). This is the pair working as
designed: breaker for *broken*, bulkhead for *slow*.
**Watch:** Resilience dashboard; webhook delivery logs show fast
backpressure rejections instead of a 30 s goroutine pile-up.

## Rules of the game

1. **One fault at a time**, in a non-production environment, with the
   dashboards open before you inject.
2. **State the hypothesis first.** "Let's see what happens" is how chaos
   becomes an outage.
3. **Automate what you learn** — a failed hypothesis becomes a regression
   test in the same commit, per the repo rule in
   `docs/testing/regression-checklist.md`.
4. **Restore and verify recovery**, not just the failure: alerts must
   resolve, breakers must close, burn rates must fall back under 1.

## What this lab is not

Not a substitute for multi-zone failure drills, not load testing (use the
k6 suite for that), and not a license to run toxics in production. The
production version of this discipline is a game day with an rollback
trigger — which is exactly what the canary analysis in
`docs/deployment/gitops.md` automates.
