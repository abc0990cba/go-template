# ADR 0016 — A Circuit Breaker Around Every Outbound Dependency

**Status:** Accepted

## Context

The api and worker call four kinds of things that can hang or fail:
external HTTP APIs (enrichment), consumer endpoints (webhooks), and two
brokers (Kafka produce, NATS publish). Without isolation, one slow
dependency consumes goroutines, balloons latency percentiles, and turns
its outage into the whole system's outage.

## Decision

A tiny in-repo breaker registry (`internal/reliability`): per named
dependency, **open after 5 consecutive failures, stay open 30 s, admit
one probe, close on success, re-open on probe failure**. Context
cancellation never counts as a failure. Every transition emits
`app_cb_state{name}` (0/1/2) and `app_cb_transitions_total` — Grafana
sees breaker health without parsing logs. Calls through an open breaker
fail fast with a mapped `503 dependency temporarily unavailable`.

Named breakers in use: `enrichment.http`, `webhooks.http`,
`kafka.produce`, `nats.publish`. The registry tolerates nil (optional
wiring runs the call directly).

## Consequences

**Positive:** failure isolation is visible and automatic; half-open
probing gives self-healing without operator action; the whole
implementation is ~150 readable lines — a teaching artifact, not a
framework.

**Negative:** consecutive-failure counting (rather than sliding-window
ratio) can trip on 5 sporadic failures among successes — accepted at
template scale, tunable via `APP_CB_FAILURE_THRESHOLD`; breakers protect
callers, not the callee (rate limiting is the callee-side concern).

## Alternatives considered

- *sony/gobreaker / failsiverse* — fine libraries; the in-repo version is
  preferred for a template whose goal is showing the pattern.
- *Per-request timeouts only* — necessary, not sufficient: timeouts
  without breakers shed nothing.
- *Bulkheads (per-dependency worker pools)* — the natural next step if a
  product grows; the breaker `Do(ctx, fn)` seam is where it would slot in.
