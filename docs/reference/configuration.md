# Configuration Reference

Every tunable in this template is a plain environment variable. There are no
config files, no YAML/JSON/TOML to parse, and no config library: the binaries
call `os.Getenv` through a single package (`internal/config`), validate every
value, and fail fast. For local development, copy `.env.example` to `.env`;
containers and schedulers receive the same variables through `environment:`
maps (Docker Compose), Nomad job `env` blocks, or Kubernetes env/Secret mounts.

Related pages: [Quickstart](../getting-started/quickstart.md),
[Development workflow](../getting-started/development-workflow.md),
[Deployment](../deployment/orchestrators.md),
[Reliability operations](../operations/reliability.md).

## How loading works

**Strict validation.** Every numeric, duration, boolean, and enum value is
parsed and range-checked at startup. A malformed value aborts the process with
an error naming the variable — it never silently falls back to the default.
For example, `APP_MAX_CONCURRENCY=eight` stops the api with:

```text
APP_MAX_CONCURRENCY: strconv.ParseInt: parsing "eight": invalid syntax
```

Beyond parse errors, cross-field rules are enforced at startup:

| Rule | Error |
| --- | --- |
| `APP_MAX_CONCURRENCY`, `APP_SHUTDOWN_TIMEOUT`, `APP_EXTERNAL_TIMEOUT`, `OAUTH_STATE_TTL`, `APP_SESSION_TTL`, `APP_IDEMPOTENCY_TTL`, `APP_RETENTION_PROCESSED_EVENTS`, `APP_FILES_MAX_MB`, outbox/circuit-breaker/bulkhead settings | must be positive |
| `APP_OTLP_SAMPLER` | must be in `(0, 1]` |
| `APP_PG_MIN_CONNS` vs `APP_PG_MAX_CONNS` | min must not exceed max |
| `APP_JWT_SECRET` | at least 32 bytes |
| `APP_JWT_REFRESH_TTL` vs `APP_JWT_ACCESS_TTL` | refresh must not be shorter than access |
| `APP_EMAIL_MODE=queue` | requires `APP_REDIS_ADDR` |
| `APP_S3_ENDPOINT` set without `APP_S3_BUCKET` | bucket required |
| `APP_JWT_SECRETS_FILE` set | file must exist, be readable, and be non-empty |
| `APP_LOCKOUT_THRESHOLD`, `APP_LOADSHED_MAX_INFLIGHT`, `APP_REQUEST_TIMEOUT`, `APP_AUDIT_RETENTION`, `APP_PG_*` counts | must not be negative (0 = feature off or driver default) |

**Value formats.**

- Durations use Go syntax: `300ms`, `3s`, `15m`, `720h`, `168h`.
- Booleans (`APP_COOKIE_SECURE`, `APP_S3_USE_PATH_STYLE`) use
  `strconv.ParseBool`: `1`/`t`/`T`/`true`/`TRUE`/`True` and `0`/`f`/`F`/`false`
  etc. Anything else (e.g. `yes`) fails startup. Two flags are literal string
  comparisons instead: `APP_PPROF` and `APP_DEMO_SEED` are on only when set to
  exactly `1`; any other value means off (no error).
- `APP_KAFKA_BROKERS` is a comma-separated list; entries are trimmed, empty
  entries dropped.
- Enums are validated: `APP_LOG_FORMAT` (`text|json`), `APP_HIBP_MODE`
  (`off|warn|enforce`), `APP_OPENAPI_VALIDATION` (`off|requests|all`),
  `APP_EMAIL_MODE` (`sync|queue`), `APP_LOG_LEVEL` (slog levels:
  `debug|info|warn|error`).

**Empty means off.** Variables that enable optional infrastructure (database,
Redis, ClickHouse, brokers, SMTP, S3, tracing, metrics listener) default to
the empty string, which disables that feature and degrades to a safe
in-process or logged fallback. Setting the variable turns the integration on.

## Secrets: the `_FILE` pattern

Sensitive values can point at a file instead of living in the environment.
For each variable below, the loader accepts either `VAR` (direct value) or
`VAR_FILE` (path to a file whose contents — trimmed of trailing CR/LF — are
the value). **Setting both is a startup error**
(`VAR and VAR_FILE are mutually exclusive`). The pattern exists because
Docker Swarm, Kubernetes, and Nomad all mount secrets as files on disk rather
than exposing them as plain env vars, and env vars leak into `docker inspect`,
crash dumps, and process listings.

| Variable | File variant |
| --- | --- |
| `APP_DATABASE_URL` | `APP_DATABASE_URL_FILE` |
| `APP_CLICKHOUSE_PASSWORD` | `APP_CLICKHOUSE_PASSWORD_FILE` |
| `APP_SMTP_PASSWORD` | `APP_SMTP_PASSWORD_FILE` |
| `APP_JWT_SECRET` | `APP_JWT_SECRET_FILE` |
| `APP_S3_ACCESS_KEY_ID` | `APP_S3_ACCESS_KEY_ID_FILE` |
| `APP_S3_SECRET_ACCESS_KEY` | `APP_S3_SECRET_ACCESS_KEY_FILE` |

`APP_JWT_SECRETS_FILE` is inherently a file-only variable (the JWT rotation
keyset — see below), and the migration binary (`cmd/migrate`) also honors
`APP_DATABASE_URL_FILE` directly. The production Nomad job mounts its secrets
under `/mnt/secrets` and wires exactly these variables; the Helm chart mounts
any `secrets.data` key ending in `_file` the same way.

---

## Variable reference

Defaults below are the values used when a variable is unset **or empty**, as
implemented in `internal/config/config_load.go` (not copied from
`.env.example`, which is an opinionated preset).

### HTTP server

| Variable | Default | Description |
| --- | --- | --- |
| `APP_HTTP_ADDR` | `:8080` | Listen address of the api. Also read directly by the container `HEALTHCHECK` (it probes `http://127.0.0.1<addr>/health/live`). |
| `APP_BASE_URL` | `http://localhost:8080` | Public origin. Used to build OAuth redirect URLs (`<base>/auth/<provider>/callback`) and absolute links in transactional email (invitations, email verification). |
| `APP_METRICS_ADDR` | *(empty)* | Address for a dedicated `/metrics` listener. The api serves metrics on its main port when unset; the worker has no other listener, so set this (compose uses `:9090`) for worker stream/outbox/dead-letter gauges. |

### Logging

| Variable | Default | Description |
| --- | --- | --- |
| `APP_LOG_LEVEL` | `info` | slog level: `debug`, `info`, `warn`, `error`. Invalid values fail startup. |
| `APP_LOG_FORMAT` | `text` | `text` (dev) or `json` (containers/log shippers). Lines get PII redaction and trace/span IDs stamped automatically. |

### Tracing and profiling

| Variable | Default | Description |
| --- | --- | --- |
| `APP_OTLP_ENDPOINT` | *(empty)* | OTLP/HTTP endpoint for traces, e.g. `localhost:4318` (Jaeger UI on `:16686` in the compose `observability` profile). Empty disables tracing. |
| `APP_OTLP_SAMPLER` | `1` | Head-sampling ratio, must be in `(0, 1]`. `1` keeps every trace (dev); production typically runs `0.05`–`0.25`. |
| `APP_PYROSCOPE_URL` | *(empty)* | Pyroscope endpoint (e.g. `pyroscope:4040` in compose) enabling continuous profiling. Empty keeps the profiler off. |
| `APP_PPROF` | off | Set to exactly `1` to mount `/debug/pprof`. Any other value is off. |

### Reliability

| Variable | Default | Description |
| --- | --- | --- |
| `APP_SHUTDOWN_TIMEOUT` | `10s` | Graceful-drain window on SIGTERM; must be positive. |
| `APP_EXTERNAL_TIMEOUT` | `3s` | Timeout for outbound HTTP calls (enrichment, webhooks); must be positive. |
| `APP_MAX_CONCURRENCY` | `8` | Cap on concurrent outbound work; must be positive. |

### PostgreSQL

`APP_DATABASE_URL` empty keeps the in-memory development store (data lost on
restart). Pool knobs follow pgxpool; zero values keep the driver defaults.
`APP_PG_STATEMENT_TIMEOUT` / `APP_PG_IDLE_TX_TIMEOUT` are applied server-side
by Postgres as `statement_timeout` / `idle_in_transaction_session_timeout`,
protecting the pool from runaway queries; migrations are exempt.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_DATABASE_URL` | *(empty)* | Postgres connection string, e.g. `postgres://app:app@localhost:55432/app?sslmode=disable`. `_FILE` variant supported. |
| `APP_PG_MAX_CONNS` | `0` (driver default) | Max pool connections; must not be negative. |
| `APP_PG_MIN_CONNS` | `0` (driver default) | Min idle connections; must not exceed `APP_PG_MAX_CONNS`. |
| `APP_PG_CONN_MAX_LIFETIME` | `0` (driver default) | Max age of a pooled connection. |
| `APP_PG_CONN_MAX_IDLE_TIME` | `0` (driver default) | Idle time before a connection is closed. |
| `APP_PG_HEALTH_CHECK_PERIOD` | `0` (driver default) | Pool health-check interval. |
| `APP_PG_STATEMENT_TIMEOUT` | `0` (not applied) | Server-side `statement_timeout`. |
| `APP_PG_IDLE_TX_TIMEOUT` | `0` (not applied) | Server-side `idle_in_transaction_session_timeout`. |

### ClickHouse

Analytics event warehouse. Empty `APP_CLICKHOUSE_ADDR` disables event
persistence entirely.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_CLICKHOUSE_ADDR` | *(empty)* | Native-protocol `host:port` (compose: `clickhouse:9000`; host side published on `:9000`). |
| `APP_CLICKHOUSE_DB` | `app` | Database name. |
| `APP_CLICKHOUSE_USER` | `default` | User name. |
| `APP_CLICKHOUSE_PASSWORD` | *(empty)* | Password. `_FILE` variant supported. |

### Redis and queue

| Variable | Default | Description |
| --- | --- | --- |
| `APP_REDIS_ADDR` | *(empty)* | Redis `host:port`. Empty keeps background jobs in-process (single-binary dev mode) and rate limiting in memory; set it to share the realtime bridge, asynq queue, and distributed rate-limit buckets. |
| `APP_REPORT_CRON` | *(empty)* | Cron expression scheduling the periodic report job in the worker, e.g. `0 3 * * *`. Empty disables the schedule. |

### JWT and API tokens

Token grants for non-browser clients are opt-in: with no secret and no keyset
the `/api/auth/token` endpoints answer 501.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_JWT_SECRET` | *(empty)* | Signing secret; minimum 32 bytes. `_FILE` variant supported. Tokens minted before a keyset existed keep verifying against this while it is set. |
| `APP_JWT_SECRETS_FILE` | *(empty)* | Path to a JSON kid→secret map for zero-downtime rotation: `{"active":"2026-10","keys":{"2026-09":"<old>","2026-10":"<new>"}}`. New tokens carry `kid=active`; old kids verify until dropped from the file. File must be non-empty. |
| `APP_JWT_ACCESS_TTL` | `15m` | Access-token lifetime; must be positive. |
| `APP_JWT_REFRESH_TTL` | `720h` (30d) | Refresh-token lifetime; must be ≥ `APP_JWT_ACCESS_TTL`. |

### Sessions and cookies

| Variable | Default | Description |
| --- | --- | --- |
| `APP_SESSION_TTL` | `168h` (7d) | Session lifetime; must be positive. |
| `APP_COOKIE_SECURE` | `true` | Emit `Secure` cookies. Browsers allow Secure cookies on localhost over http; set `0` only for plain http on non-localhost hosts (compose does this for its local port). |

### Rate limiting

Three token buckets. Per-IP budgets are backed by Redis when
`APP_REDIS_ADDR` is set, in memory otherwise. All values must be positive.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_AUTH_RATE_RPS` | `5` | Per-IP rate for auth endpoints (deliberately tight — brute-force budget). |
| `APP_AUTH_RATE_BURST` | `10` | Per-IP burst for auth endpoints. |
| `APP_API_RATE_RPS` | `50` | Per-IP rate for the whole `/api`. |
| `APP_API_RATE_BURST` | `100` | Per-IP burst for `/api`. |
| `APP_USER_RATE_RPS` | `100` | Per-principal rate, keyed by user id or API-key subject — one hot account cannot exhaust the budget shared behind a NAT IP. |
| `APP_USER_RATE_BURST` | `200` | Per-principal burst. |

### Demo seed

Development convenience; leave everything here empty in production.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_DEMO_EMAIL` | *(empty)* | Demo account seeded at api startup when set (compose and the local Nomad job set `demo@go-template.local`). |
| `APP_DEMO_PASSWORD` | *(empty)* | Password for the demo account. |
| `APP_DEMO_SEED` | off | Set to exactly `1` to seed the richer demo dataset (120 users, jobs, reports, event history). Requires `APP_DATABASE_URL`; idempotent — skips when data is present. |

### OAuth providers

All providers are optional; an empty client id disables that provider.
Linking a provider requires an active session.

| Variable | Default | Description |
| --- | --- | --- |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | *(empty)* | Google OAuth credentials. |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | *(empty)* | GitHub OAuth credentials. |
| `FACEBOOK_CLIENT_ID` / `FACEBOOK_CLIENT_SECRET` | *(empty)* | Facebook OAuth credentials. |
| `TWITTER_CLIENT_ID` / `TWITTER_CLIENT_SECRET` | *(empty)* | Twitter OAuth credentials. |
| `OAUTH_STATE_TTL` | `10m` | Lifetime of the anti-CSRF state parameter; must be positive. |

Redirect URLs derive from `APP_BASE_URL` (`<base>/auth/<provider>/callback`).

### OAuth endpoint overrides

For tests against a mock provider. All default to *(empty)* = the provider's
production URLs.

| Variable | Overrides |
| --- | --- |
| `GOOGLE_AUTH_URL` / `GOOGLE_TOKEN_URL` / `GOOGLE_USERINFO_URL` | Google auth, token, userinfo endpoints. |
| `GITHUB_AUTH_URL` / `GITHUB_TOKEN_URL` / `GITHUB_USER_URL` / `GITHUB_EMAIL_URL` | GitHub auth, token, user, email endpoints. |
| `FACEBOOK_AUTH_URL` / `FACEBOOK_TOKEN_URL` / `FACEBOOK_USERINFO_URL` | Facebook auth, token, userinfo endpoints. |
| `TWITTER_AUTH_URL` / `TWITTER_TOKEN_URL` / `TWITTER_USERINFO_URL` | Twitter auth, token, userinfo endpoints. |

### SMTP and email

Without `APP_SMTP_HOST` the app logs messages instead of sending (compose
ships Mailpit — read mail at `http://localhost:8025`, SMTP on `:1025`).

| Variable | Default | Description |
| --- | --- | --- |
| `APP_SMTP_HOST` | *(empty)* | SMTP server host. Empty selects the logging mailer. |
| `APP_SMTP_PORT` | `1025` | SMTP port (Mailpit in compose; production relays usually want `587` — the Ansible default). |
| `APP_SMTP_USERNAME` | *(empty)* | SMTP username. |
| `APP_SMTP_PASSWORD` | *(empty)* | SMTP password. `_FILE` variant supported. |
| `APP_SMTP_FROM` | `no-reply@go-template.local` | From address. |
| `APP_EMAIL_MODE` | `sync` | `sync` sends inline before the request returns; `queue` records the message and the worker delivers it with retries/backoff (requires `APP_REDIS_ADDR`). Both record every message in the delivery log (`GET /api/emails` for admins). |
| `APP_EMAIL_RETENTION` | `720h` (30d) | Delivery-log retention; the worker sweeps older rows hourly. Must be positive. |

### Audit and data retention

| Variable | Default | Description |
| --- | --- | --- |
| `APP_AUDIT_RETENTION` | `8760h` (1y) | Audit-trail retention; `0` keeps entries forever. Audit entries are append-only (DB-enforced: no UPDATE/DELETE for the app role); pruning runs through the `audit_prune` SECURITY DEFINER function on the worker schedule. |
| `APP_RETENTION_PROCESSED_EVENTS` | `720h` (30d) | Retention for the exactly-once inbox (`processed_events`), which doubles as the dedupe table. Keep it longer than any redelivery delay your brokers can produce. (Export ZIPs expire after 7 days; ClickHouse events carry a 90-day TTL — those are not configurable here.) |

### Kafka and NATS streams

User enrichment from two external systems. Both sides are optional; an empty
value disables that consumer (and the outbox relay) with a startup warning.
Compose overlay `docker-compose.brokers.yml` starts Redpanda (Kafka API) and
NATS JetStream — containers reach them at `redpanda:9092` /
`nats://nats:4222`, host-side tools at `localhost:19092` / `localhost:4222`.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_KAFKA_BROKERS` | *(empty)* | Comma-separated broker list, e.g. `localhost:19092`. Empty disables the Kafka consumer and relay. |
| `APP_KAFKA_TOPIC_PROFILE` | `user-profile.v1` | Topic consumed for profile enrichment. |
| `APP_KAFKA_TOPIC_EVENTS` | `user-events.v1` | Topic consumed for user events. |
| `APP_KAFKA_GROUP` | `go-template-enricher` | Consumer group id. |
| `APP_NATS_URL` | *(empty)* | NATS URL, e.g. `nats://localhost:4222`. Empty disables the NATS consumer. |
| `APP_NATS_STREAM` | `USERS` | JetStream stream name. |
| `APP_NATS_SUBJECT_CONTACT` | `users.contact.v1` | Subject consumed for contact enrichment. |
| `APP_NATS_SUBJECT_EVENTS` | `users.events.v1` | Subject consumed for user events. |
| `APP_NATS_DURABLE` | `enricher-contact` | JetStream durable consumer name. |

### Circuit breaker and bulkhead

| Variable | Default | Description |
| --- | --- | --- |
| `APP_CB_FAILURE_THRESHOLD` | `5` | Consecutive failures before a breaker opens (wraps enrichment HTTP, webhooks, broker publishes); must be positive. |
| `APP_CB_OPEN_TIMEOUT` | `30s` | Cool-down before the half-open probe; must be positive. |
| `APP_BULKHEAD_LIMIT` | `16` | Max concurrent calls per outbound dependency; a full bulkhead rejects fast (backpressure) instead of stacking goroutines. Rejections never trip the breaker. Must be positive. |
| `APP_BULKHEAD_ACQUIRE_TIMEOUT` | `1s` | How long a call waits for a bulkhead slot; must be positive. |

### Outbox relay

Worker-side drain of transactional events to the brokers. All must be
positive.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_OUTBOX_INTERVAL` | `1s` | Drain interval. |
| `APP_OUTBOX_BATCH` | `100` | Events drained per pass. |
| `APP_OUTBOX_MAX_ATTEMPTS` | `10` | Attempt budget before an event lands in dead letters. |

### Idempotency

| Variable | Default | Description |
| --- | --- | --- |
| `APP_IDEMPOTENCY_TTL` | `24h` | How long stored `Idempotency-Key` responses stay replayable; must be positive. |

### Breached passwords (HIBP)

Checks run against the Have I Been Pwned corpus via the k-anonymity range
API — only a 5-char SHA-1 prefix of the password ever leaves the process.
Checks fail open when the API is unreachable (availability over strictness).

| Variable | Default | Description |
| --- | --- | --- |
| `APP_HIBP_MODE` | `off` | `off` disables checks; `warn` logs compromised passwords and allows them; `enforce` rejects them at registration and password reset. |
| `APP_HIBP_URL` | `https://api.pwnedpasswords.com` | Range API base URL (tests point it at a stub). |

### Progressive lockout

After `APP_LOCKOUT_THRESHOLD` wrong passwords the account locks for
`APP_LOCKOUT_WINDOW`, doubling per further failure up to a 24h cap. A
successful login or password reset clears it; admins can unlock via
`POST /api/users/{id}/unlock`. Login answers identically for locked and
wrong-password accounts.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_LOCKOUT_THRESHOLD` | `5` | Failures before lockout; `0` disables the feature. |
| `APP_LOCKOUT_WINDOW` | `15m` | Base lock duration; must be positive. |

### OpenAPI validation

Runtime contract enforcement on `/api` against the embedded OpenAPI spec (the
same file `/openapi.yaml` serves).

| Variable | Default | Description |
| --- | --- | --- |
| `APP_OPENAPI_VALIDATION` | `requests` | `off` disables the gate; `requests` rejects non-conforming requests with 400 `problem+json`; `all` additionally validates responses (mismatches are logged, never altered). |

### Load shedding

| Variable | Default | Description |
| --- | --- | --- |
| `APP_LOADSHED_MAX_INFLIGHT` | `512` | Cap on concurrent `/api` requests; `0` disables. Bulk surfaces (reports, analytics, exports) shed at 3/4 of the cap, keeping a reserve lane for interactive traffic; shed requests get 503 + `Retry-After`. |

### Timeouts

| Variable | Default | Description |
| --- | --- | --- |
| `APP_REQUEST_TIMEOUT` | `30s` | Per-request context deadline on `/api` (`0` disables; must not be negative). Streaming and file transfers are exempt. The deadline propagates to database and queue calls, so a wedged backend surfaces as a fast 504 instead of a hung connection. |

### S3 and file storage

Without `APP_S3_BUCKET` uploads land in the in-process store (dev only; a
restart loses them). S3 mode activates when either `APP_S3_ENDPOINT` or
`APP_S3_BUCKET` is set — the bucket then becomes mandatory, and a failed
reachability probe degrades back to the in-process store with a warning.
Empty `APP_S3_ENDPOINT` with a bucket means real AWS S3; with an endpoint,
MinIO or any S3-compatible store (compose `--profile files` runs MinIO on
`localhost:19000`).

| Variable | Default | Description |
| --- | --- | --- |
| `APP_S3_ENDPOINT` | *(empty)* | Full URL including scheme, e.g. `http://localhost:19000`. Empty = AWS S3. |
| `APP_S3_REGION` | `us-east-1` | Bucket region (read only when S3 mode is on). |
| `APP_S3_BUCKET` | *(empty)* | Bucket name; setting it (or the endpoint) switches storage to S3. |
| `APP_S3_ACCESS_KEY_ID` | *(empty)* | Static access key. `_FILE` variant supported. |
| `APP_S3_SECRET_ACCESS_KEY` | *(empty)* | Static secret key. `_FILE` variant supported. |
| `APP_S3_USE_PATH_STYLE` | `true` | Path-style addressing (MinIO wants it on). |
| `APP_FILES_MAX_MB` | `10` | Max single-upload size in MiB; converted to bytes internally. Must be positive. |

### Worker

| Variable | Default | Description |
| --- | --- | --- |
| `APP_METRICS_ADDR` | *(empty)* | Worker-only: address for its own `/metrics` (stream, outbox, dead-letter gauges live in the worker process). Compose sets `:9090` (published on host `:19090` with the brokers overlay); Helm uses `:9091`; the Nomad job binds `127.0.0.1:9090`. The api ignores it and serves metrics on its main port. |

The worker also consumes everything needed to produce exports and drain the
outbox: `APP_DATABASE_URL`, `APP_REDIS_ADDR`, `APP_CLICKHOUSE_*`,
`APP_SMTP_*`, `APP_S3_*`, `APP_REPORT_CRON`, and the stream settings above.

---

## Who sets what where

The same variable names are used everywhere; only the transport differs
(`environment:` map, `.env` file, Nomad `env` block, Kubernetes env/Secret).
Sources: `docker-compose.yml`, `docker-compose.brokers.yml`,
`deploy/ansible/inventories/prod/group_vars/vps/main.yml` (+ vault-encrypted
`secrets.yml`), `deploy/nomad/app.nomad.hcl`, `deploy/helm/go-template/values.yaml`.

| Variable | docker-compose.yml | .env (local) | Ansible VPS → Nomad | Helm values |
| --- | --- | --- | --- | --- |
| `APP_DATABASE_URL` | `postgres:5432` in-network | `localhost:55432` (compose-published port) | `APP_DATABASE_URL_FILE=/mnt/secrets/database_url` (secret from vault `secrets.yml`) | `config.databaseURL`, or `secrets.data` with `_FILE` key |
| `APP_REDIS_ADDR` | `redis:6379` | `localhost:6379` | `127.0.0.1:6379` (co-located) | `config.redisAddr` |
| `APP_CLICKHOUSE_ADDR` | `clickhouse:9000` | `localhost:9000` or unset | unset (feature off) | `config.clickhouseAddr` |
| `APP_JWT_SECRET` | unset (token endpoints off) | optional (dev only) | vault `jwt_secret` → `APP_JWT_SECRET_FILE=/mnt/secrets/jwt_secret` | `secrets.data` (`_FILE` mount) |
| `APP_SMTP_HOST` / `_PORT` / `_USERNAME` / `_FROM` | pass-through `${APP_SMTP_HOST:-}` etc. | optional | `app_smtp_*` in `group_vars` (port default `587`, from `no-reply@<domain>`) → Nomad `var.smtp_*` | `config.extraEnv` |
| `APP_SMTP_PASSWORD` | unset (logging mailer) | optional | vault → `APP_SMTP_PASSWORD_FILE=/mnt/secrets/smtp_password` | `secrets.data` |
| `APP_COOKIE_SECURE` | `0` (local plain http) | `1` (localhost is exempt in browsers) | `1` | `config.cookieSecure: "1"` |
| `APP_LOG_FORMAT` / `APP_LOG_LEVEL` | `json` / unset (`info`) | `text` / as you like | `json` / `info` | `json` / unset |
| `APP_OTLP_ENDPOINT` | `jaeger:4318` | `localhost:4318` with the observability profile | unset in prod job (local Nomad job: `host.docker.internal:4318`) | `config.otlpEndpoint` |
| `APP_KAFKA_BROKERS` / `APP_NATS_URL` | brokers overlay: `redpanda:9092` / `nats://nats:4222` | `localhost:19092` / `nats://localhost:4222` | unset | `config.extraEnv` |
| `APP_METRICS_ADDR` | worker `:9090` | unset | worker `127.0.0.1:9090` | `worker.metricsAddr` (`:9091`) |
| `APP_REPORT_CRON` | worker `0 3 * * *` | optional | `app_report_cron` → `var.report_cron` | `config.extraEnv` |
| `APP_DEMO_EMAIL` / `_PASSWORD` / `_SEED` | set, `SEED=1` | optional | only in the **local rehearsal** Nomad job, never prod | unset |
| `APP_PG_MAX_CONNS`, `APP_API_RATE_RPS` | unset (driver/rate defaults) | unset | unset | `api.env` (`"20"`, `"50"`) |
| `APP_OUTBOX_INTERVAL` | unset (`1s`) | unset | unset | `worker.env` (`1s`) |
| OAuth client id/secret | unset | optional | vault `secrets.yml` (`app_oauth_google` etc.) | `secrets.data` |
| `APP_BASE_URL` | unset (`http://localhost:8080`) | as needed | `var.base_url` from `app_domain` | unset or `config.extraEnv` |

Rules of thumb visible in the matrix: compose hardcodes in-network addresses
and demo credentials; Ansible/Helm/Nomad inject only non-secret tuning from
`group_vars`/`values.yaml` and route every secret through the `_FILE` mount
pattern; anything not listed reaches production via Helm `config.extraEnv`
or a Nomad job edit.

## Example configurations

### (a) Minimal local dev — in-memory store, zero infrastructure

Copy to `.env`. No Postgres, no Redis, no brokers: jobs run in-process, files
live in memory, mail is logged, streams are off. Only the demo account is a
convenience add.

```bash
APP_HTTP_ADDR=:8080
APP_BASE_URL=http://localhost:8080
APP_LOG_LEVEL=debug
APP_LOG_FORMAT=text
APP_COOKIE_SECURE=1            # browsers allow Secure cookies on localhost
# No APP_DATABASE_URL / APP_REDIS_ADDR -> in-memory store, in-process jobs.
APP_DEMO_EMAIL=demo@go-template.local
APP_DEMO_PASSWORD=demo-password-123
```

### (b) Hybrid dev — Postgres + Redis on localhost via compose

Start the backing services from the repo root (`docker compose up -d
postgres redis`, plus profiles as needed), then point your host-run api/worker
at the published ports. Note Postgres is published on **55432** (host) — it
deliberately differs from 5432 to dodge local port-forwarder quirks.

```bash
APP_HTTP_ADDR=:8080
APP_BASE_URL=http://localhost:8080
APP_LOG_LEVEL=info
APP_LOG_FORMAT=text
APP_COOKIE_SECURE=1

APP_DATABASE_URL=postgres://app:app@localhost:55432/app?sslmode=disable
APP_REDIS_ADDR=localhost:6379
APP_DEMO_SEED=1                # richer dataset; requires the DB above
APP_DEMO_EMAIL=demo@go-template.local
APP_DEMO_PASSWORD=demo-password-123

# Optional add-ons, each behind its own compose profile:
APP_CLICKHOUSE_ADDR=localhost:9000            # analytics events
APP_OTLP_ENDPOINT=localhost:4318              # traces -> Jaeger UI :16686
APP_KAFKA_BROKERS=localhost:19092             # brokers overlay (make demo-up)
APP_NATS_URL=nats://localhost:4222
APP_S3_ENDPOINT=http://localhost:19000        # --profile files (MinIO)
APP_S3_BUCKET=app-files
APP_S3_ACCESS_KEY_ID=filesapp
APP_S3_SECRET_ACCESS_KEY=files-secret
APP_SMTP_HOST=localhost                       # --profile mail (Mailpit)
APP_SMTP_PORT=1025
APP_EMAIL_MODE=queue                          # worker delivers; needs Redis
```

### (c) Production-ish — security-relevant settings

Secrets come from mounted files (Docker/K8s/Nomad secret mounts), never
plain env. Demo seeding stays unset. The exact wiring for Nomad/Helm is in
`deploy/nomad/app.nomad.hcl` and `deploy/helm/go-template/values.yaml`.

```bash
APP_HTTP_ADDR=:8080
APP_BASE_URL=https://app.example.com          # drives OAuth redirects + email links
APP_LOG_LEVEL=info
APP_LOG_FORMAT=json
APP_COOKIE_SECURE=1

# Secrets via the _FILE pattern (mutually exclusive with the direct value)
APP_DATABASE_URL_FILE=/mnt/secrets/database_url
APP_JWT_SECRET_FILE=/mnt/secrets/jwt_secret   # >= 32 bytes
APP_SMTP_PASSWORD_FILE=/mnt/secrets/smtp_password

APP_REDIS_ADDR=127.0.0.1:6379
APP_REPORT_CRON=0 3 * * *
APP_PG_MAX_CONNS=20
APP_PG_STATEMENT_TIMEOUT=30s

APP_SMTP_HOST=smtp.example.com
APP_SMTP_PORT=587
APP_SMTP_USERNAME=no-reply@example.com
APP_SMTP_FROM=no-reply@example.com
APP_EMAIL_MODE=queue                          # worker delivers with retries

APP_OTLP_ENDPOINT=otel-collector:4318
APP_OTLP_SAMPLER=0.1

# Breached-password + lockout hardening
APP_HIBP_MODE=enforce
APP_LOCKOUT_THRESHOLD=5
APP_LOCKOUT_WINDOW=15m

# Managed S3 (real AWS: leave APP_S3_ENDPOINT empty)
APP_S3_BUCKET=app-files-prod
APP_S3_REGION=eu-central-1
APP_S3_ACCESS_KEY_ID_FILE=/mnt/secrets/s3_access_key
APP_S3_SECRET_ACCESS_KEY_FILE=/mnt/secrets/s3_secret_key

# Keep traffic shaping at defaults, or tune:
APP_LOADSHED_MAX_INFLIGHT=512
APP_REQUEST_TIMEOUT=30s
APP_OPENAPI_VALIDATION=requests
```

## Deprecated or unused variables

None. Every variable in `.env.example` is read by `internal/config` (or, for
`APP_HTTP_ADDR` and `APP_DATABASE_URL_FILE`, additionally by the api
healthcheck and the migration binary). Two easy-to-miss entries are commented
out in `.env.example` but fully supported: `APP_JWT_SECRETS_FILE` (JWT key
rotation) and `APP_HIBP_URL` (HIBP endpoint override).

Note that `.env.example` is a preset, not a list of defaults — it turns
several features **on** that the code leaves off by default
(`APP_DATABASE_URL`, `APP_REDIS_ADDR`, `APP_CLICKHOUSE_ADDR`,
`APP_KAFKA_BROKERS`, `APP_DEMO_SEED`, `APP_REPORT_CRON`). When the file and
this document disagree on a default, `config_load.go` wins.
