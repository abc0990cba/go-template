# ADR 0023 — Log↔Trace Correlation, Configurable Sampling, and Continuous Profiling

**Status:** Accepted

## Context

Tracing existed end-to-end (ADR-level: HTTP → asynq → Kafka/NATS → pgx),
but three practical gaps kept the signals apart:

1. **Logs and traces did not reference each other.** Log lines carried
   `request_id` only; finding the trace for a log line (or the logs for a
   trace) meant manual time-correlation.
2. **Sampling was always-on.** At template traffic that is fine; at
   production traffic it is the most expensive telemetry mistake — 100%
   of traces paid for at full volume, most of them boring.
3. **Latency had no "why".** Metrics say *that* p95 regressed; only
   profiles say *which function* is burning CPU or allocating. pprof
   (on-demand, `APP_PPROF=1`) answers nothing when nobody is looking.

## Decision

- **Every log line inside a span carries `trace_id`/`span_id`.** A
  `slog.Handler` wrapper (`observ.NewTraceHandler`) appends the IDs from
  the record's context; both binaries build their logger through it. The
  reverse direction: the access-log middleware stamps `request.id` onto
  the server span. With tracing disabled the wrapper adds nothing —
  correlation is free at zero cost.
- **`APP_OTLP_SAMPLER` (0..1]** selects head sampling via
  `ParentBased(TraceIDRatioBased)`: child spans (including async workers
  receiving propagated context) keep the root's decision. Validation
  rejects out-of-range values at startup; the sampler itself falls back
  to always-sample rather than silently dropping everything.
- **The remaining boundaries got spans:** outbound HTTP (webhook,
  enrichment, OAuth) through `otelhttp.NewTransport`; Redis cache /
  limiter / pub-sub clients through `redisotel`; ClickHouse through the
  driver's explicit `WithSpan` propagation (the driver does not read
  contexts implicitly — the adapter attaches the span context per query).
- **Continuous profiling** via Pyroscope behind `APP_PYROSCOPE_URL`
  (off by default): CPU + heap profiles pushed every cycle, flame graphs
  in Grafana next to the metrics. Profiling failure degrades to no-op.

## Consequences

**Positive:** the three signals — logs, metrics, traces — plus profiles
cross-reference: alert → dashboard → trace → log line → flame graph is
one continuous path; sampling makes tracing cost proportional to value;
memory-adapter modes (no Redis, no ClickHouse) lose nothing.

**Negative:** the `TraceHandler` wrapper adds two attrs per line inside
spans (~40 bytes in JSON) — negligible; `redisotel` cannot cover asynq's
internal Redis pool (queue latency is visible via job-duration metrics
and asynq's own Web UI instead); profiling adds a background uploader
(~1-2% CPU) — why it stays opt-in.

## Alternatives considered

- *slog-otel handler libraries* — the wrapper is ~50 lines and exact;
  a dependency to save 50 lines fails the template's taste test.
- *Tail-based sampling* (keep slow/error traces only) — strictly better
  signal-per-byte but needs the OTel Collector with tail processing;
  named as the production next step in `docs/observability.md`.
- *eBPF profiling (Grafana Beyla/otel-profiles)* — zero-instrumentation
  and fashionable, but the pull-based Go SDK keeps the teaching surface
  inside the code the reader already owns.
