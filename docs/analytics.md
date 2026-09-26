# Analytics

ClickHouse is planned for append-only analytics events.

Schema:

- `migrations/clickhouse/001_events.sql`

Events:

- `user_registered`
- `user_logged_in`
- `oauth_linked`
- `job_queued`
- `job_done`
- `job_failed`

PostgreSQL remains the operational source of truth. ClickHouse is for dashboards and reports.
