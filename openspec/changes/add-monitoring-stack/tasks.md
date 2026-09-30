## 1. Dashboards
- [x] 1.1 Dashboard provisioning provider + compose mount of `deploy/grafana/dashboards`
- [x] 1.2 api-overview, worker-jobs, resilience dashboards over existing `app_*` metrics

## 2. Alerts And SLOs
- [x] 2.1 `alerts.yml` cause/symptom rules; `prometheus.yml` rule_files + alertmanager target
- [x] 2.2 `slo.yml` recording rules + multi-window burn alerts; Alertmanager config + service
- [x] 2.3 blackbox-exporter service + probe scrape job + `LiveProbeFailed` alert

## 3. Docs
- [x] 3.1 `docs/slo.md`; ADR 0022; observability docs updates
