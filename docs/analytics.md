# Analytics

ClickHouse is the append-only event warehouse; PostgreSQL stays the operational source of truth. When `APP_CLICKHOUSE_ADDR` is configured, the service applies `migrations/clickhouse/*.sql` at startup, batches named events into `analytics_events` (flush on batch size or interval; remaining events flush on shutdown), and the dashboard merges event counters into the summary. If the warehouse is down, the dashboard still serves the operational counters — analytics never breaks the request path.

Event names: `user_registered`, `user_logged_in`, `oauth_linked`, `job_queued`, `job_done`, `job_failed`.

Code map: sink + aggregates in `internal/platform/clickhouse`, wiring in `internal/bootstrap`, dashboard merge in `internal/service/analytics.go`.

Rules:

- Events are append-only; never update or delete rows in the warehouse.
- Producers must not fail when the warehouse is unavailable — drops are logged.
- Batch inserts only (buffer + flush), never one insert per event on the request path.
- Operational counters keep coming from the store; warehouse counters are additive, `omitempty` in the API contract.
