## Why

Metrics were exported and scraped, but the watching half of observability was missing: no dashboards, no alert rules, no Alertmanager, no SLO layer, and no outside-in check. A template teaching observability has to close the emit/watch loop (ADR 0022).

## What Changes

- **Dashboards as code**: `deploy/grafana/provisioning/dashboards/provider.yml` + three provisioned dashboards (API overview RED + runtime; Worker & queues incl. outbox/DLQ; Resilience incl. breakers/rate limits/idempotency), all reading existing `app_*` metrics.
- **Alerting**: `deploy/prometheus/rules/alerts.yml` (process up, HTTP error/latency, jobs failing, outbox backlog, DLQ pending, breaker open, limiter fail-open, blackbox probe) loaded via `rule_files`; Alertmanager service in the observability profile with a placeholder receiver and documented Slack/PagerDuty recipes.
- **SLO layer**: `rules/slo.yml` recording rules + multi-window burn-rate alerts (99.9% availability, 95% under 500ms — the k6 budgets); `docs/slo.md` explains budget math and symptom-vs-cause alerting.
- **Synthetic checks**: blackbox-exporter probing `/health/live` from outside the process.
- Worker `/metrics` enabled in base compose (Prometheus scraped a dead endpoint without the brokers overlay).

## Capabilities

### New Capabilities

- `monitoring`

## Impact

- Deploy/config only (dashboards, rules, compose services); no application code changes. Stable datasource UIDs (`prometheus`, `jaeger`, `pyroscope`) are referenced by dashboards and the Rollouts AnalysisTemplate.
