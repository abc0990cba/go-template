# deploy/prometheus/ — alert & SLO rules

Loaded by Prometheus alongside the root scrape config at `deploy/prometheus.yml`.

- `rules/alerts.yml` — 20 alerting rules over API, worker, queue/stream and infrastructure signals.
- `rules/slo.yml` — recording rules (multi-window request/error/latency rates) plus fast- and slow-burn alerts: `SLOAvailabilityFastBurn`/`SlowBurn`, `SLOLatencyFastBurn`/`SlowBurn`.

Runbooks per alert: [../../docs/operations/runbooks/alerts.md](../../docs/operations/runbooks/alerts.md)
