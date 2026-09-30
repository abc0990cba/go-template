## Purpose

The observability capability keeps the three signals (logs, metrics, traces) plus profiles cross-referenced and cost-proportional: any alert leads to a dashboard, then a trace, then log lines, then a flame graph.

## ADDED Requirements

### Requirement: Logs Carry Trace Context

Every log line emitted inside an active span shall include `trace_id` and `span_id`; the server span shall carry the `request.id` attribute so traces point back at their log lines.

#### Scenario: One lookup path

- **WHEN** an operator finds a slow request in Jaeger
- **THEN** its `request.id` matches the `request_id` field of the corresponding log lines

### Requirement: Sampling Is Configurable

Head sampling shall follow `APP_OTLP_SAMPLER` (0..1] as a parent-based ratio; out-of-range values fail startup.

#### Scenario: Production ratio

- **WHEN** `APP_OTLP_SAMPLER=0.1`
- **THEN** roughly one in ten root traces is exported and their children keep the decision

### Requirement: Every Boundary Spans

Outbound HTTP, Redis commands, ClickHouse queries, and Postgres queries shall appear as spans under the trace that caused them.

#### Scenario: Webhook delivery

- **WHEN** a webhook is delivered in response to an API request
- **THEN** the trace contains the server span and an HTTP client child span for the delivery

### Requirement: Pool Saturation Is Observable

pgxpool statistics shall be exported as `app_pgpool_*` metrics, including cumulative waits for a connection.

#### Scenario: Pool exhaustion

- **WHEN** all pool connections are checked out under load
- **THEN** `app_pgpool_waiters_total` climbs while `app_pgpool_acquired_conns` pins at `app_pgpool_max_conns`

### Requirement: Continuous Profiling Is Opt-In

Setting `APP_PYROSCOPE_URL` shall push CPU and heap profiles to Pyroscope; unset keeps the process free of background uploaders.

#### Scenario: Flame graph after a regression

- **WHEN** a deploy regresses CPU and profiling is on
- **THEN** the Pyroscope datasource shows the hot function within one upload cycle
