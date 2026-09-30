## Purpose

The monitoring capability turns the exported metrics into a watched system: dashboards, alert rules, SLO burn-rate gates, and synthetic probes are versioned artifacts, provisioned automatically, and reviewed in PRs like code.

## ADDED Requirements

### Requirement: Dashboards Are Code

Grafana dashboards shall be provisioned from files in the repository (no hand-built snowflakes), covering API RED + runtime, worker queues (jobs, outbox, dead letters), and the resilience machinery (breakers, rate limits, idempotency), using only metrics the binaries already export.

#### Scenario: Fresh observability profile

- **WHEN** `docker compose --profile observability up` starts
- **THEN** the three dashboards appear under the "Go Template" folder without UI actions

### Requirement: Alerts Fire Somewhere

Prometheus shall evaluate alert rules from `deploy/prometheus/rules/` and route firing alerts through Alertmanager; every rule shall carry a summary and a first-diagnostic-step description.

#### Scenario: Breaker trips

- **WHEN** `app_cb_state{name}` equals 2 for two minutes
- **THEN** `BreakerOpen` fires with severity warning and is visible in the Alertmanager UI

### Requirement: SLO Burn Gates

Availability (99.9%) and latency (95% < 500ms) SLOs shall be recorded as burn rates and alerted on multi-window (fast 14.4x, slow 6x), so pages correspond to budget burn rather than raw errors.

#### Scenario: Fast burn

- **WHEN** the 5m and 1h burn rates both exceed 14.4x
- **THEN** `SLOAvailabilityFastBurn` fires as critical

### Requirement: Outside-In Probe

A blackbox exporter shall probe `/health/live` as a real client, and its failure shall alert independently of whitebox metrics.

#### Scenario: Router broken, process alive

- **WHEN** the api process runs but HTTP requests never reach it
- **THEN** `LiveProbeFailed` fires while whitebox app metrics look green
