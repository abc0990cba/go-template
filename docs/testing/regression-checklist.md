# Regression Checklist

The full verification pass for the template: automated gauntlet plus a live
feature walkthrough. Run it before cutting a release, after rebasing onto
 unfamiliar ground, or whenever something feels off. Everything here runs
locally with zero external dependencies — compose profiles and make targets
cover every feature (see the local-runnability matrix at the bottom).

## 0. Start the stack

```bash
make docker-up                                   # postgres, redis, api, worker
docker compose --profile observability up -d     # jaeger, prometheus, grafana (optional)
docker compose --profile mail up -d              # mailpit — reads invitation/verification email
docker compose --profile files up -d             # minio — S3 mode for files/exports (optional)
```

Hybrid dev mode (faster iteration) instead of the compose api:

```bash
docker compose up -d postgres redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_DEMO_EMAIL="demo@go-template.local" APP_DEMO_PASSWORD="demo-password-123" \
APP_DEMO_SEED=1 APP_COOKIE_SECURE=0 go run ./cmd/api
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" go run ./cmd/worker
```

For the e2e suite use a **fresh** database (the first registered account
becomes superadmin; seeded stacks skip those tests by design):

```bash
docker exec go-template-postgres-1 psql -U app -d app -c "DROP DATABASE IF EXISTS e2e" -c "CREATE DATABASE e2e"
```

## 1. Automated gauntlet

| # | Command | Green means |
|---|---|---|
| 1 | `make generate && git diff --exit-code` | OpenAPI ↔ `api.gen.go` and sqlc ↔ queries are in sync |
| 2 | `make vet` + `gofmt -l internal cmd` (empty) | vet clean, formatted |
| 3 | `make lint` | golangci-lint (v2 config) clean |
| 4 | `make test` | race-enabled unit suite green |
| 5 | `make coverage` | coverage reported (no floor gate; do not drop it) |
| 6 | `make test-integration` | contract tests vs real Postgres/Redis/Kafka/NATS/ClickHouse (+ MinIO where pullable; skips gracefully otherwise) |
| 7 | `GO_TEMPLATE_BASE_URL=… go test -tags e2e ./test/e2e/` | live happy path, health, RBAC (needs fresh DB + worker) |
| 8 | `k6 run -e BASE_URL=… test/performance/smoke.js` | 100% checks, p95 < 500ms |
| 9 | `make web-lint web-test web-build` | SPA lint/type/tests/build |
| 10 | `make helm-lint helm-template` | chart lints and renders |

## 2. Live walkthrough (all features)

`scripts/regression-walkthrough.sh` drives every new capability against a
running stack and prints ✓/✗ per step (33 checks):

```bash
APP_JWT_SECRET="local-secret-0123456789abcdef" go run ./cmd/api   # token grants are opt-in
BASE=http://localhost:8080 bash scripts/regression-walkthrough.sh
```

Covered, in order:

1. **Auth bootstrap** — demo admin login.
2. **MFA** — enroll (secret + otpauth), verify-enable, login returns the
   challenge, challenge completes with a TOTP, disable (password + code).
   Manual extras: scan the QR from Profile → Two-factor (authenticator app
   or totp.app), use a recovery code once, watch the second use fail, admin
   reset (`DELETE /api/users/{id}/mfa`) clears the factor.
3. **JWT token grants** — password grant → access token works on /api/users,
   refresh rotates, presenting the retired token revokes the whole family,
   revoke is idempotent. Manual: decode the access JWT — identity claims
   only, no roles.
4. **Org invitations** — admin invites an email with a role, the invitee
   accepts, the token is single-use. Manual extras: the invitation email
   lands in Mailpit (http://localhost:8025) with the accept link;
   `/accept-invitation` page; the invitee sees it under
   `GET /api/invitations`.
5. **Files + ticket attachments** — multipart upload, byte-exact download,
   attach to a ticket (timeline entry + notification), attachment listed,
   presign 501 in memory mode, owner-only delete.
6. **GDPR export** — request (202), job completes (queue worker or
   `POST /api/jobs/process` in single-process mode), download is a ZIP with
   per-entity JSON. Multi-process note: the api can only stream the bundle
   with `APP_S3_*` configured (MinIO via `--profile files`).
7. **Search + ETag** — `GET /api/search?q=` returns permission-scoped
   matches; list responses carry strong ETags and answer 304 to
   `If-None-Match`.
8. **Maintenance mode** — `maintenance.enabled` flips the API to 503 with
   `Retry-After`, health stays green, the settings API stays reachable, and
   flipping it back restores service within the memo window.
9. **Per-identity rate limiting** — hammering /api/me as one user yields 429
   (default budget `APP_USER_RATE_RPS=100`, burst 200).
10. **Observability** — `/metrics` serves the RED + business metrics,
    `/version` reports build info; with the observability profile, Jaeger
    shows server spans per request (route-named) joined with DB/consumer
    spans.

## 3. Local-runnability matrix

| Feature | What you need | Where to look |
|---|---|---|
| MFA | nothing (default run) | Profile → Two-factor; login challenge |
| JWT grants | `APP_JWT_SECRET` | `POST /api/auth/token` |
| Invitations | `--profile mail` for the email (or the echoed token) | Orgs → Seats → Invite |
| Files/uploads | `--profile files` for persistence; memory store works in-process | `POST /api/files` |
| Ticket attachments | nothing | ticket detail → attachments |
| Data export | queue worker (+ `--profile files`/S3 in queue mode) | `POST /api/users/me/export` |
| Search | nothing (Postgres tsvector or memory fallback) | `GET /api/search?q=` |
| ETag | nothing | any listed GET |
| Maintenance mode | nothing (Admin → Settings) | 503 + `Retry-After` |
| Per-identity limits | nothing (tune `APP_USER_RATE_*`) | 429 + `Retry-After` |
| Traces | `--profile observability` + `APP_OTLP_ENDPOINT` | Jaeger :16686 |
| Streams/outbox/DLQ | `make demo-up` + `make demo-enrichment` / `demo-dlq` | Streams admin page |
| Email flows | `--profile mail` | Mailpit :8025 |

## 4. After any failure

Fix the root cause (not the symptom), re-run the specific step, then re-run
the **whole** automated gauntlet before committing. Every bug found during a
regression pass gets a regression test in the same commit.
