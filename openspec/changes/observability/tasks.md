## 1. Logging

- [x] 1.1 APP_LOG_FORMAT text|json in both binaries
- [x] 1.2 accessLog middleware: request id, status, duration; slow-request warn

## 2. Metrics

- [x] 2.1 RED middleware (counter, histogram, in-flight) with route-pattern labels
- [x] 2.2 service.JobObserver port + Prometheus adapter; jobs queued/processed/duration
- [x] 2.3 /metrics endpoint; runtime metrics

## 3. Tracing

- [x] 3.1 observ.SetupTracing: OTLP/HTTP exporter, resource with build version
- [x] 3.2 otelgin server spans; asynq header propagation both directions
- [x] 3.3 otelpgx query tracing via bootstrap option

## 4. Profiling, Version, Stack

- [x] 4.1 pprof behind APP_PPROF; /version with ldflags build info
- [x] 4.2 compose observability profile: Jaeger + Prometheus + Grafana, deploy configs
- [x] 4.3 tests: /metrics contents, /version default; docs/observability.md
