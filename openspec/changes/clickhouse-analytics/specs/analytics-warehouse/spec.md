## Purpose

The analytics warehouse persists the named event trail in ClickHouse and feeds dashboard aggregates without ever endangering the request path.

## ADDED Requirements

### Requirement: Batched Event Persistence

When `APP_CLICKHOUSE_ADDR` is configured, named events shall be batched and inserted into `analytics_events`, with buffered events flushed on shutdown.

#### Scenario: Registration events survive a restart

- **WHEN** users register and the process shuts down gracefully
- **THEN** the `user_registered` events are present in the warehouse afterwards

### Requirement: Graceful Degradation

A warehouse outage shall degrade the dashboard to operational counters instead of failing requests.

#### Scenario: Warehouse unreachable

- **WHEN** ClickHouse is unavailable and a client requests the analytics summary
- **THEN** the response succeeds with the operational counters and the outage is logged

### Requirement: Aggregate Dashboard

The dashboard shall merge per-event counters from the warehouse into the analytics summary when the warehouse is reachable.

#### Scenario: Dashboard after activity

- **WHEN** events have been persisted and the dashboard is opened
- **THEN** the event-trail panel shows counters matching the warehouse
