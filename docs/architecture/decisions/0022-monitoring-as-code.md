# ADR 0022 — Monitoring as Code: Dashboards, Alerts, and SLOs in the Repo

**Status:** Accepted

## Context

Metrics were exported (`app_*` catalog) and scraped by Prometheus, but the
loop was open: no dashboards existed (doc 12 "left to the adopter"), no
alert rules fired anywhere, and no Alertmanager was deployed. Operators
were expected to hand-build queries after the first incident — the exact
moment nobody should be writing PromQL for the first time. A template
that teaches observability has to ship the watching half, not just the
emitting half.

## Decision

The full watching stack is versioned in `deploy/` and provisioned
automatically by the `observability` compose profile:

- **Dashboards as code** (`deploy/grafana/dashboards/*.json`, provider in
  `provisioning/dashboards/`): API overview (RED + Go runtime), Worker &
  queues (jobs, outbox, DLQ), Resilience (breakers, rate limits,
  idempotency). Every panel reads existing `app_*` metrics — dashboards
  ship in the same PR as the metric they visualize.
- **Alert rules** (`deploy/prometheus/rules/alerts.yml`): symptom-level
  SLO burn alerts plus cause-level alerts for the machinery the template
  already has (outbox backlog, DLQ depth, breaker open, limiter
  fail-open, blackbox probe). Every alert carries a summary and a
  description that names the first diagnostic step.
- **SLO layer** (`rules/slo.yml` + `docs/slo.md`): 99.9% availability and
  95%-under-500ms latency — the same latency number k6 enforces — with
  multi-window burn-rate alerts (14.4x page / 6x ticket).
- **Alertmanager** in the observability profile; routing config with a
  harmless placeholder receiver and commented Slack/PagerDuty/email
  recipes. Firing alerts are always visible in its UI.
- **Synthetic check**: blackbox-exporter probing `/health/live` from the
  outside — the whitebox `/metrics` view cannot catch a broken router or
  ingress.

Datasources and dashboards get stable UIDs (`prometheus`, `jaeger`,
`pyroscope`) so panels and the Rollouts AnalysisTemplate can reference
them.

## Consequences

**Positive:** a fresh `docker compose --profile observability up` yields
a complete observe→alert loop with zero UI clicking; alert thresholds and
dashboard changes go through PR review like any other code; the
symptom-vs-cause split teaches alerting discipline instead of "alert on
everything".

**Negative:** dashboards drift with the metric catalog and must be
updated when metrics change (same PR rule keeps this honest); template
thresholds are guesses until a deployment baselines real traffic —
documented in `docs/slo.md`; the Alertmanager receiver is a placeholder
by design, adopters must wire real delivery before trusting paging.

## Alternatives considered

- *Leave dashboards to adopters* (status quo) — rejected: the emit/watch
  asymmetry made the observability story half-true.
- *Grafana Foundation SDK (dashboards as Go code)* — attractive for
  type-safe panels, but JSON under `deploy/` is inspectable without a Go
  toolchain and matches how most teams store dashboards today.
- *Sloth / Perses SLO generators* — fine tooling; hand-written
  multi-window PromQL is more instructive for a template whose goal is
  showing the pattern.
