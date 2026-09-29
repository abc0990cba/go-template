# ADR 0017 — ClickHouse as a Separate OLAP Store; OLTP Stays in Postgres

**Status:** Accepted

## Context

Product analytics (event firehose: registrations, logins, ticket actions,
page views — with 90-day charts, DAU, dimension breakdowns) is a scan-
heavy, append-only workload. Postgres can do it at small scale but bloats
tables and competes with transactional traffic. The template wants to
demonstrate the OLTP/OLAP split correctly, not pretend one store fits
all.

## Decision

Events flow through the `EventSink` port into **ClickHouse**
(`analytics_events`, MergeTree ordered by `(event_name, occurred_at)`,
`LowCardinality` names, JSON properties string) via a **buffered batch
writer** (500 events or 2 s flush; write failures are logged, never
propagated — analytics must not break checkout, so to speak). All
dashboard reads hit ClickHouse. **Fallback:** without ClickHouse the
same port is served by the in-memory trail, so dashboards degrade
instead of disappearing. Domain writes (tickets, users) never join
against the warehouse — the `jobs`/`tickets` tables remain the
transactional truth.

## Consequences

**Positive:** analytic scans stop touching OLTP; the write path is
async and lossy-tolerant by design; columnar storage makes the
timeseries/breakdown queries one-liners.

**Negative:** two databases to operate (the compose/Helm stacks carry
it); near-real-time only (2 s flush); no joins between events and domain
rows (by design — event properties carry what dashboards need); ClickHouse
TTL/retention is intentionally unset, left as adopter policy.

## Alternatives considered

- *Postgres partitioned tables for events* — fine up to tens of millions
  of rows; the split becomes mandatory later and is painful then.
- *TimescaleDB* — keeps one engine; the template prefers demonstrating
  the explicit OLAP boundary (and ClickHouse's columnar performance is
  categorically better for breakdowns).
- *Kafka → ClickHouse without the API path* — more moving parts in dev;
  the buffered writer keeps zero-dependency mode alive.
