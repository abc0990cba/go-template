# Analytics

ClickHouse is the append-only event warehouse; PostgreSQL stays the operational source of truth. When `APP_CLICKHOUSE_ADDR` is configured, the service applies `migrations/clickhouse/*.sql` at startup, batches named events into `analytics_events` (flush on batch size or interval; remaining events flush on shutdown), and the dashboard merges event counters into the summary. If the warehouse is down, the dashboard still serves the operational counters — analytics never breaks the request path.

Event names: `user_registered`, `user_logged_in`, `oauth_linked`, `job_queued`, `job_done`, `job_failed`.

Code map: sink + aggregates in `internal/platform/clickhouse`, wiring in `internal/bootstrap`, dashboard merge in `internal/service/analytics.go`.

Rules:

- Events are append-only; never update or delete rows in the warehouse.
- Producers must not fail when the warehouse is unavailable — drops are logged.
- Batch inserts only (buffer + flush), never one insert per event on the request path.
- Operational counters keep coming from the store; warehouse counters are additive, `omitempty` in the API contract.

## Reading the warehouse (SPA endpoints)

- `GET /api/analytics/timeseries?days=30` — per-day, per-event counts for charts (ClickHouse aggregate; in-memory trail in dev mode).
- `GET /api/analytics/events?limit=50` — newest trail records with the acting `user_id`.
- `GET /api/events/stream` — Server-Sent Events: `analytics.event` (emitted through the realtime fan sink the moment an event is recorded), `job.updated` (worker transitions relayed over Redis pub/sub), and a ~5 s `snapshot` (summary + newest jobs) that keeps browsers converging even after a missed message.

Events now carry `user_id` end-to-end: the `EventSink` signature is `AddEvent(ctx, name, userID)`, so warehouse rows attribute registrations, logins, and OAuth links to accounts — which is what makes the seeded 30-day history renderable per user.

Demo data: `APP_DEMO_SEED=1` (or `make seed-demo`) backfills 30 days of realistic events via `InsertEvents` batch inserts; `make seed-demo-reset` removes them (`DeleteDemoEvents` issues a ClickHouse mutation, asynchronous by design).
