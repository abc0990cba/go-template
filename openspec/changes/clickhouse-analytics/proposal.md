## Why

Events were recorded only in the in-memory store (or dropped entirely on PostgreSQL), so the analytics trail disappeared on restart and never left the process.

## What Changes

- Add a ClickHouse adapter: connection, startup migration application, a batching event sink (flush by size or interval, final flush on shutdown), and event-count aggregates.
- Split the ports: `EventSink` (write) and `EventStats` (read) replace the `AddEvent` methods embedded in the auth/job store ports; services emit through an injected sink, keeping the domain warehouse-agnostic.
- `AnalyticsService.Summary` merges warehouse counters into the operational summary; a warehouse outage logs and degrades to operational counters only.
- Dashboard gains an event-trail panel; the contract adds optional `events_*` fields to `AnalyticsSummary`.
- compose/env wire ClickHouse for api and worker; integration tests cover the sink→flush→aggregate round trip and migration idempotency (skip when the image is unavailable).

## Capabilities

### New Capabilities

- `analytics-warehouse`

## Impact

- With `APP_CLICKHOUSE_ADDR` set, events survive restarts and are visible across processes; without it, behavior is unchanged.
