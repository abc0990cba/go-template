# deploy/alertmanager/ — alert routing

- `alertmanager.yml` — the routing tree: grouped by `alertname`/`severity`, with a faster, more frequent route for `severity="critical"`.

**Warning:** the default receivers (`local-sink`, `critical-sink`) are harmless placeholders that webhook to a dead endpoint — alerts stay visible in the Alertmanager UI (localhost:9093) but go nowhere else. Configure a real receiver before relying on this. Commented recipes for Slack, PagerDuty and email are inside the file.

Runbooks: [../../docs/operations/runbooks/alerts.md](../../docs/operations/runbooks/alerts.md)
