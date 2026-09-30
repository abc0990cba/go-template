## 1. Correlation And Sampling
- [x] 1.1 `observ.TraceHandler` + both mains + unit tests
- [x] 1.2 `APP_OTLP_SAMPLER` config + `samplerFor` + validation tests
- [x] 1.3 accessLog sets `request.id` span attribute

## 2. Boundaries
- [x] 2.1 otelhttp on webhook/enrichment/OAuth clients
- [x] 2.2 redisotel on api redis client + realtime bridge
- [x] 2.3 ClickHouse `tracedCtx` span propagation at every query site

## 3. Metrics And Profiling
- [x] 3.1 `PoolStatsCollector` + `Store.Stats()` + bootstrap wiring + registration in both binaries
- [x] 3.2 Pyroscope service, datasource, `observ.StartProfiler` behind `APP_PYROSCOPE_URL`

## 4. Docs
- [x] 4.1 ADR 0023; `.env.example`; observability docs updates
