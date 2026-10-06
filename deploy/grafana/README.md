# deploy/grafana/ — provisioned Grafana

Drop-in provisioning; mounts directly as Grafana's config volumes.

- `provisioning/datasources/datasources.yml` — Prometheus, Jaeger (traces), Pyroscope (profiles).
- `provisioning/dashboards/provider.yml` — loads dashboards from the folder below.
- `dashboards/` — `api-overview.json`, `worker-jobs.json`, `resilience.json`.

Docs: [../../docs/operations/observability.md](../../docs/operations/observability.md)
