# CLI tools

The `cmd/` tree builds six standalone binaries: the two long-running
services (`api`, `worker`) and four operational tools (`migrate`, `seed`,
`seed-demo`, `devtools`).

The tools exist as separate binaries because the production image is
distroless: no shell, no package manager, no way to `kubectl exec ... -- psql`.
Schema migrations, seeding, and stream debugging are therefore either run
from your workstation against the target database, or shipped as one-shot
jobs that run the same binary the platform already trusts.

All four tools read configuration the same way the services do — `APP_*`
environment variables, with `APP_DATABASE_URL` (or the mounted-secret
pattern `APP_DATABASE_URL_FILE`) standing in for the database connection.

## cmd/migrate

Applies the embedded PostgreSQL migrations outside the app's boot path.
Three uses, one per subcommand:

```bash
migrate up                 # apply pending migrations, print the resulting version
migrate version            # print schema version and dirty flag
migrate force <version>    # force the version (e.g. -1) to clear a dirty state
```

- Connection string comes from `APP_DATABASE_URL` or `APP_DATABASE_URL_FILE`;
  the binary exits with an error if neither is set.
- It walks `migrations/postgres` **on disk** — the same directory the app
  embeds at boot, so both paths always ship one schema.
- Use `up` when you want migrations applied before the app starts (CI gates
  run it against a staging database), `version` to verify what a deploy
  actually landed, and `force` to recover a migration that failed mid-flight
  (a dirty version always needs `force` plus human attention).

Versus auto-migrate at boot: the API still migrates on startup as a
fallback, and the pgx5 driver takes a Postgres advisory lock for the
migration's duration, so concurrent callers (boot on N replicas, `cmd/migrate`
racing a deploy) serialize instead of racing. Prefer the explicit binary for
production deploys — it makes the schema change a visible, gated step.

Convenience wrappers live in the Makefile (`make migrate`, `make
migrate-version`, `make migrate-force N=-1`); see
[make-targets](make-targets.md#database-migrations).

## cmd/seed

Perf-data seeder for the k6 scenarios. Requires `APP_DATABASE_URL` and
refuses to run without one.

```bash
go run ./cmd/seed -users 100000    # or: make seed-users USERS=100000
```

| Flag | Default | Meaning |
|---|---|---|
| `-users` | `100000` | Number of users to insert |

What it creates:

- `users` rows `perf-0`, `perf-1`, ... with emails `perf-N@example.com`,
  bulk-inserted with pgx `COPY`; `created_at` steps backwards one second per
  user so keyset pagination pages through real variety.
- One `password_credentials` row and one `password` auth identity per user,
  all with the password `password123` — enough for the k6 login scenarios to
  authenticate against a freshly seeded database.

Argon2id hashing note: the password hash is computed **once** and shared by
every seeded user. The real hashing cost is ~20 ms per hash, which would add
~33 minutes to a 100k seed if hashed individually. The hash mirrors
`internal/auth` exactly, so credentials verify against the real login path.

Runtime: the whole run is bounded by a 10-minute context; a 100k seed
finishes well inside it.

## cmd/seed-demo

Loads the demo dataset so the SPA has realistic content to explore.
Requires `APP_DATABASE_URL`.

```bash
go run ./cmd/seed-demo           # or: make seed-demo
go run ./cmd/seed-demo -reset    # or: make seed-demo-reset
```

| Flag | Default | Meaning |
|---|---|---|
| `-reset` | `false` | Delete everything the seed created (demo users cascade their credentials, identities, and sessions) before reseeding |

Without `-reset` the seed is idempotent: every row carries the `demo-` ID
prefix and the first user (`demo-u-0001`) doubles as the marker — if it
exists, the seed is skipped.

The dataset (from `internal/bootstrap/demodata.go`, fixed RNG seed 42, so
runs are reproducible):

| What | Count | Notes |
|---|---|---|
| Users | 500 | Spread over 90 days; ids `demo-u-0001`...`demo-u-0500`, external_ref = id |
| Loginable users | 10 | `demo-u-0001`...`demo-u-0010`, password `password123`, email = `<id>@demo.example` |
| Named accounts | 6 | Ada Admin, Max Manager, Vera Viewer, Rita Requester, Ari Approver, Sam Staff (the login-page table) |
| OAuth identities | ~1 in 4 users | Google (every 4th), GitHub (every 8th) |
| Jobs | 160 | Weighted across done/failed/queued/running |
| Reports | 40 | — |
| Enrichment profiles | 220 | — |
| Tickets | 120 | Across the whole ticket state machine, plus ticket types |
| Organizations | 3 | With demo roles and org-scoped policies |
| Warehouse events | 90 days | Backfilled event history (ClickHouse when configured) |
| Audit entries | 200 | — |
| Notifications | 60 | — |
| API keys / webhooks | 2 / 8 | — |

`APP_DEMO_*` interplay: the same seed runs inside the API process when
`APP_DEMO_SEED=1` (that is also the only way to seed the in-memory store,
since `cmd/seed-demo` exits if `APP_DATABASE_URL` is empty). The defaults in
`.env.example` also set `APP_DEMO_EMAIL` / `APP_DEMO_PASSWORD`
(`demo@go-template.local` / `demo-password-123`), which the devtools and
`scripts/regression-walkthrough.sh` use to log in.

## cmd/devtools

The local testing toolbox for the enrichment streams and resilience
features. It publishes events into Kafka and NATS, watches what the backend
processed, and drives the breaker / rate-limit / idempotency demos; each
scenario prints what to check in the app. Full walkthrough:
[enrichment streams](../guides/enrichment-streams.md).

Flags have sensible defaults for `make demo-up` (Redpanda on
`localhost:19092`, NATS on `localhost:4222`, API on `localhost:8080`), with
environment fallbacks `KAFKA_BROKERS`, `NATS_URL`, `API_BASE`,
`DEMO_EMAIL`, `DEMO_PASSWORD`. It logs in via `/api/auth/login` with the
demo credentials to call authenticated endpoints.

| Subcommand | What it does | Notable flags |
|---|---|---|
| `kafka-profile` | Publish profile event(s) to the Kafka `user-profile.v1` topic | `--ref`, `--seq`, `--count 5`, `--bad` (poison message), `--brokers`, `--topic` |
| `nats-contact` | Publish contact event(s) to NATS JetStream (`USERS` stream, `users.contact.v1`) | `--ref`, `--seq`, `--count 5`, `--bad`, `--url`, `--stream`, `--subject` |
| `consume-user-events` | Consume the `user-events.v1` topic and print `user.enriched` events (what the outbox relay published) | `--timeout 15s`, `--brokers`, `--topic` |
| `demo` | End to end: publish both streams for one user, then poll the API (up to 30s) for the merged enrichment | `--api`, `--ref` |
| `demo-dlq` | Publish a poison message, watch it land in dead letters, replay it | `--api` |
| `dlq-list` | List pending dead letters (`GET /api/streams/dead-letters`) | `--api` |
| `dlq-replay` | Replay one dead letter (`POST /api/streams/dead-letters/{id}/replay`) | `--id` (required), `--api` |
| `demo-breaker` | Drive a flaky upstream until the app's circuit breaker trips open, then recovers | watch `app_cb_state{name="enrichment.http"}` on `/metrics` |
| `flaky-server` | Tiny HTTP server that fails N times then succeeds — the raw ingredient for breaker testing | `--addr :8099`, `--failures 4` |
| `hammer` | Concurrent load until 429s come back; prints the `Retry-After` value | `--requests 200`, `--concurrency 10`, `--target` |

Copy-paste examples (what the `demo-*` Makefile targets run under the hood):

```bash
go run ./cmd/devtools kafka-profile --ref demo-u-0001 --seq 1 --company "Acme Corp"
go run ./cmd/devtools nats-contact  --ref demo-u-0001 --seq 1
go run ./cmd/devtools demo          --ref demo-u-0001
go run ./cmd/devtools demo-dlq
go run ./cmd/devtools dlq-list
go run ./cmd/devtools dlq-replay --id <dead-letter-id>
go run ./cmd/devtools consume-user-events --timeout 15s
go run ./cmd/devtools hammer --requests 200 --concurrency 10
```

Build artifact note: the compiled binary at the repo root (`/devtools`) is
gitignored. There is no dedicated `make devtools` target — the `demo-*`
targets use `go run` directly — but you can build it yourself:

```bash
go build -o devtools ./cmd/devtools
./devtools demo --ref demo-u-0001
```

## The binaries at a glance

| Binary | Kind | Purpose |
|---|---|---|
| `cmd/api` | Service | HTTP server: JSON API, embedded SPA, SSE stream, health endpoints; graceful shutdown on SIGINT/SIGTERM |
| `cmd/worker` | Service | Background jobs via Asynq/Redis (local polling without Redis); also runs the Kafka/NATS enrichment consumers, the outbox relay, and idempotency-key pruning |
| `cmd/migrate` | Ops tool | Apply / inspect / force-recover PostgreSQL schema migrations |
| `cmd/seed` | Ops tool | Bulk-insert perf users + credentials for k6 scenarios |
| `cmd/seed-demo` | Ops tool | Load (or `-reset` and reload) the demo dataset for the SPA |
| `cmd/devtools` | Dev tool | Publish to the enrichment streams; DLQ, breaker, rate-limit, and idempotency scenarios |
