# Application Overview — Components & How To Verify Each By Hand

What the app is made of, where every piece lives, and the shortest way to
see each one work with your own eyes. The repeatable deep procedure
(automated gauntlet + 33-check live walkthrough) lives in
[docs/regression-checklist.md](regression-checklist.md); this page is the
map.

## The big picture

One Go binary serves the **JSON API and the embedded React SPA**; a second
binary is the **worker** (queues, stream consumers, outbox relay, exports).
Storage is PostgreSQL (pgx + sqlc, no ORM); Redis carries the queue, the
shared rate-limit/cache layer and realtime fan-out; ClickHouse is the
analytics warehouse; Kafka + NATS JetStream carry the enrichment streams;
S3/MinIO holds files. Architecture is ports & adapters
(`internal/service/ports.go` is the contract; `internal/platform/*` and
`internal/repository` are the adapters; `internal/httpapp` is the HTTP
layer). **Everything optional degrades**: no Postgres → in-memory store,
no Redis → in-process queue/rate limits, no ClickHouse → in-memory event
trail, no S3 → in-memory blobs, no SMTP → mail logged.

Start everything locally:

```bash
make docker-up        # postgres, redis, api, worker  → http://localhost:8080
```

Demo login: `demo@go-template.local` / `demo-password-123` (seeded at boot;
`APP_DEMO_SEED=1` adds the full dataset: users, orgs, 120 tickets, events).

## Components

### 1. HTTP foundation (middleware chain, in order)

`internal/httpapp/`: security headers + HSTS → request id → **server
tracing span** → Prometheus RED metrics → access log → **maintenance gate**
→ **body cap** (`MaxBytesReader`, 413) → per-IP rate limit → origin/CSRF
check → auth (cookie | `gtpl_` API key | JWT) → **per-identity rate limit**
→ org domain → permission guard → handler → ETag.

**Check by hand:**

```bash
curl -si localhost:8080/health/ready | grep -E "^(HTTP|Strict-Transport)"   # 200; HSTS when APP_COOKIE_SECURE=1
curl -si localhost:8080/api/users -H "Authorization: Bearer $T" | grep -iE "^etag|^x-request-id"
curl -s localhost:8080/metrics | grep app_http_requests_total               # RED metrics
curl -s localhost:8080/version                                              # build info
```

### 2. Auth & identity — `internal/service/auth.go`, `mfa.go`, `tokens.go`, `email.go`, `apikeys.go`

Opaque server-side sessions (Argon2id passwords, sliding TTL), OAuth
Google/GitHub/Facebook/Twitter linking, password reset / email verification (hashed
single-use tokens, SMTP or Mailpit or log), personal `gtpl_` API keys with
scope narrowing, admin impersonation (audited + notified), **TOTP MFA**
(enroll → QR → login challenge → recovery codes → `mfa.required` policy →
admin reset), **JWT token grants** (password / mfa_challenge /
refresh_token; rotation with reuse-detection families).

**Check by hand:**

```bash
# register → session cookie + bearer token
curl -si -X POST localhost:8080/api/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","name":"Me","password":"password123"}' | grep -E "^HTTP|token"
# UI: login → Profile → Two-factor → scan QR with your authenticator → sign out → sign in (code asked)
# JWT (needs APP_JWT_SECRET):
curl -s -X POST localhost:8080/api/auth/token -H 'Content-Type: application/json' \
  -d '{"grant_type":"password","email":"demo@go-template.local","password":"demo-password-123"}'
```

### 3. Authorization (multi-tenant RBAC) — `internal/service/rbac.go`, `internal/platform/rbac/`

Casbin with domains: organizations are tenants, roles carry
`resource:action` grants, `X-Org-Id` switches scope, superadmin/admin/
manager/requester/approver/viewer seeded, raw policy editor, first
registered account becomes superadmin, expanded permission set cached and
invalidated on change.

**Check by hand:** sign in as `demo-u-0002@demo.example` / `password123`
(manager) — the admin console renders, writes it may not do answer 403;
switch the org scope in the header and watch lists narrow. Roles matrix:
Admin → Roles.

### 4. Business domains

| Domain | Where | What it does |
|---|---|---|
| Users | `internal/service/users.go` | CRUD, three pagination modes (offset/cursor/keyset), per-user activity timeline |
| Jobs | `internal/service/jobs.go`, `internal/jobs/`, `internal/platform/queue/` | asynq/Redis queue + worker binary; types: enrich batch, report, **export**; live status over SSE |
| Reports & analytics | `internal/service/reports.go`, `analytics.go` | report snapshots; ClickHouse event warehouse with dashboards (fallback: in-memory trail) |
| Settings & flags | `internal/service/settings.go` | typed KV catalog with admin UI, public flags, read-through cache |
| Notifications | `internal/service/notifications.go` | in-app center, unread badge, SSE toasts, email copies |
| Webhooks | `internal/service/webhooks.go` | HMAC-SHA256 signed deliveries, retries, delivery log, test-send |
| Tickets & approvals | `internal/service/tickets.go` | state machine, multi-step approval chains, requester/approver surfaces, comments, timeline |
| **Org invitations** | `internal/service/invitations.go` | email offers, single-use tokens, accept = membership + domain role |
| **Files** | `internal/service/files.go`, `internal/platform/filestore/` | S3/MinIO or in-memory blobs, checksums, ticket attachments, presigned URLs |
| **Data export** | `internal/service/export.go` | GDPR bundle: queued job → ZIP of per-entity JSON, owner-only, 7-day retention |
| **Search** | `internal/service/search.go` | permission-scoped full-text (Postgres tsvector / memory fallback) |
| Event streams | `internal/service/streams.go`, `platform/kafka`, `platform/nats` | exactly-once enrichment (inbox + sequence + outbox), DLQ with admin replay |
| Audit | `internal/service/audit.go` | immutable who/what/when with before/after diffs, per-user history |

**Check by hand (UI first, API second):**

```bash
T="Authorization: Bearer $(curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')"
curl -s "localhost:8080/api/search?q=expense" -H "$T"                          # search (users + tickets)
curl -s -X POST localhost:8080/api/users/me/export -H "$T"                     # 202, then download:
sleep 3; curl -s -o /tmp/me.zip localhost:8080/api/users/me/export/download -H "$T" && unzip -l /tmp/me.zip
echo hi > /tmp/f.txt; curl -s -X POST localhost:8080/api/files -H "$T" -F file=@/tmp/f.txt   # upload
curl -s localhost:8080/api/audit?limit=3 -H "$T"                               # audit trail
```

UI walk: file a ticket as `demo-u-0004`, approve it as `demo-u-0002` then
`demo-u-0005` (each: password `password123`) — notifications and the
timeline update live over SSE; invite by email in Orgs → Seats (Mailpit at
:8025 with `--profile mail`); upload an attachment on the ticket detail.

### 5. Platform adapters — `internal/platform/`

`postgres` (sqlc-generated queries, migrations at boot) · `repository`
(in-memory twin, kept identical by contract tests) · `cache` (memory /
Redis) · `queue` (asynq) · `ratelimit` (memory / Redis sliding window) ·
`rbac` (Casbin enforcer) · `mailer` (SMTP / log) · `oauth` · `clickhouse` ·
`kafka` (franz-go) · `nats` (JetStream) · `filestore` (S3 / memory).

**Check by hand:** `make docker-up` boots Postgres+Redis (logs show
"postgres store ready"); run with none of the env vars and everything still
works on the in-memory store. Adapters are proved interchangeable by
`test/integration/store_contract_test.go` (same assertions against both).

### 6. Reliability toolkit — `internal/reliability/`, `internal/httpapp/idempotency.go`

Circuit breakers around every outbound call (`app_cb_state` metrics),
retry with jitter + permanent-error marker, bounded concurrency,
**Idempotency-Key** middleware replaying stored responses, transactional
outbox → brokers, dead-letter queue with audited replay.

**Check by hand:**

```bash
# idempotent ticket filing: same key twice → both answer 201 (the second IS
# the stored replay), but only one ticket exists and the counter ticks.
K="demo-key-$(date +%s)"; TYPE="<ticket-type-id>"
curl -s -X POST localhost:8080/api/tickets -H "$T" -H "Idempotency-Key: $K" -H 'Content-Type: application/json' \
  -d "{\"type_id\":\"$TYPE\",\"title\":\"Once\",\"description\":\"\"}" -o /dev/null -w "%{http_code}\n"   # 201
curl -s -X POST localhost:8080/api/tickets -H "$T" -H "Idempotency-Key: $K" -H 'Content-Type: application/json' \
  -d "{\"type_id\":\"$TYPE\",\"title\":\"Once\",\"description\":\"\"}" -o /dev/null -w "%{http_code}\n"   # 201 replay
curl -s "localhost:8080/api/tickets?q=Once" -H "$T" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']))"  # 1
curl -s localhost:8080/metrics | grep "^app_idempotency_replays_total"  # 1
# breakers/DLQ demo: make demo-up && make demo-breaker / demo-dlq (see docs/enrichment-streams.md)
```

### 7. Observability — `internal/observ/`, `internal/httpapp/observability.go`

slog (text/json), Prometheus on `/metrics` (HTTP RED, jobs, breaker states,
outbox/DLQ gauges, rate-limit rejections, idempotency replays), OpenTelemetry
OTLP → Jaeger (server spans per request joined with DB/queue/broker spans),
`/health/live|ready|startup`, `/version`, pprof behind `APP_PPROF=1`.

**Check by hand:** `docker compose --profile observability up -d` → Grafana
:3000, Prometheus :9090, Jaeger :16686 (click a trace: route span →
sqlc query spans). Metrics + health curls are in §1.

### 8. React SPA — `web/src/`

Two shells: **client app** at `/` (dashboard, my tickets, approvals,
notifications, profile with API keys + MFA) and **admin console** under
`/admin/*` (dashboard, users + activity, jobs, enrichment, reports,
analytics, audit, settings/flags, streams/outbox/DLQ, roles, policies,
orgs, ticket types, webhooks) — permission-gated per route and re-checked
server-side. EN/RU i18n. Built into the binary (`go:embed`).

**Check by hand:** open http://localhost:8080, sign in as the demo admin,
walk the sidebar; sign in as `demo-u-0004` (requester) and confirm `/admin`
redirects away; toggle EN/RU in the header.

### 9. Deployment — `Dockerfile`, `docker-compose*.yml`, `deploy/helm/`, `.github/workflows/`

Distroless multi-stage image; compose profiles `mail` (Mailpit),
`observability` (Jaeger/Prometheus/Grafana), `files` (MinIO), brokers
overlay (`make demo-up`); Helm chart (HPA/PDB/probes/secrets mounts,
`make helm-lint`); release pipeline (tag → GHCR multi-arch → Trivy → SBOM →
cosign).

**Check by hand:** `make helm-lint && helm template go-template
deploy/helm/go-template | head -40`.

## The 5-minute smoke test

```bash
make docker-up                       # or the hybrid mode from docs/quickstart.md
open http://localhost:8080           # sign in: demo@go-template.local / demo-password-123
BASE=http://localhost:8080 bash scripts/regression-walkthrough.sh   # 33 checks, ~1 min
```

`PASS=33 FAIL=0` is the whole app verified live: auth, MFA, JWT grants,
invitations, files, export, search, ETag, maintenance mode, rate limits,
metrics. Anything red → `docs/regression-checklist.md` §4.
