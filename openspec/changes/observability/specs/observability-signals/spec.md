## Purpose

Observability signals define how the service explains itself in production: what happened (logs), how much and how fast (metrics), and where time went (traces).

## ADDED Requirements

### Requirement: Structured Request Logs

Every HTTP request shall produce one structured log line containing method, route, status, duration, and request id; slow requests shall be escalated.

#### Scenario: JSON logs in compose

- **WHEN** the api runs with `APP_LOG_FORMAT=json`
- **THEN** each request logs a single JSON object parseable by a log shipper

### Requirement: RED Metrics

The service shall expose request count, duration histogram, in-flight gauge, and job lifecycle counters on `/metrics` with bounded label cardinality.

#### Scenario: Metrics endpoint

- **WHEN** `/metrics` is scraped after serving traffic
- **THEN** `app_http_requests_total` and `app_http_request_duration_seconds` are present and non-zero

### Requirement: Cross-Process Tracing

With `APP_OTLP_ENDPOINT` set, HTTP requests, database queries, and queue tasks shall carry OpenTelemetry spans, and a task's spans shall join the trace of the request that enqueued it.

#### Scenario: Enqueue to execution

- **WHEN** the API enqueues a job while tracing is enabled and the worker executes it
- **THEN** the worker span references the API span as its parent via propagated task headers
