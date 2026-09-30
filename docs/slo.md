# Service Level Objectives

The template ships with working SLO machinery, not just a suggestion:
recording rules, multi-window burn-rate alerts, and dashboards that show
the error budget. Numbers live in `deploy/prometheus/rules/slo.yml`; the
reasoning lives here.

## The objectives

| SLO | Target | Rationale |
|---|---|---|
| Availability | 99.9% of requests return status < 500 over 30 days | 43m 50s of total 5xx downtime per month. Tight enough to bite, loose enough that a solo team can hold it without heroics. |
| Latency | 95% of requests complete in < 500 ms over 30 days | The same budget the k6 suite enforces in CI (`docs/performance.md`); one number for tests and operations. |

Status 4xx is deliberately *not* an error: a 404 or 429 is the product
working (bad request, shedding load), not the service failing.

## The error budget

`budget = 1 − target`. For availability that is 0.1% of requests —
the number you may spend on badness before users are measurably
suffering. Everything operational follows from spending it well:

- **Budget full** → ship features, take risks, deploy on Friday.
- **Budget draining** → slow down: freeze risky deploys, fix the top
  offender, add capacity.
- **Budget empty** → reliability work only, until the 30-day window
  refills.

Prometheus computes burn rate for you:

```
burn = (current error ratio) / (allowed error ratio)
```

Burn 1.0 means "exactly on budget". Burn 14.4 means "at this rate the
30-day budget is gone in ~2 days".

## Multi-window burn alerts (why not "alert on every 5xx")

Raw error alerts flap; percentage alerts miss slow leaks. The classic
compromise (Google SRE Workbook, ch. 5) fires only when **two windows
agree**:

| Alert | Windows | Burn | Budget impact | Severity |
|---|---|---|---|---|
| `SLOAvailabilityFastBurn` | 5 m + 1 h | > 14.4 | 30-day budget in ~2 days | critical (page) |
| `SLOAvailabilitySlowBurn` | 30 m + 6 h | > 6 | budget in ~5 days | warning (ticket) |
| `SLOLatencyFastBurn` / `SlowBurn` | same | same | latency budget | critical / warning |

The short window keeps time-to-detect low; the long window kills
flapping. Both rules are plain PromQL in `slo.yml` — read them, they are
the teachable part.

## Where to see it

- **Grafana → "Go Template / API overview"**: error ratio and p95 against
  the thresholds.
- **Prometheus → Alerts** (localhost:9090/alerts): every rule, its state
  (`ok / pending / firing`), and the burn-rate recording values.
- **Alertmanager UI** (localhost:9093): what is firing, what is silenced,
  and where it would be routed.

## Symptom vs cause alerts

The burn alerts are *symptoms* — what users feel. The rules in
`alerts.yml` are mostly *causes* (outbox backlog, breaker open, limiter
fail-open). Keep the distinction when you add alerts:

- Page on symptoms (users hurt now).
- Ticket on causes (something will hurt later).
- A cause without a symptom alert is a candidate dashboard panel, not a
  page — the system may be absorbing it exactly as designed (that is
  what the breakers and DLQ are for).

## Adopting this for real

1. Measure first: run a month with the recording rules and *no* paging;
   adjust targets to what your traffic actually does.
2. Wire a real receiver in `deploy/alertmanager/alertmanager.yml` (Slack /
   PagerDuty / email configs are sketched in comments there).
3. Give every alert a runbook link in its annotations (the shipped alerts
   point at dashboards and `docs/*.md` instead — template default).
4. Review budgets in your ops retro; change targets via PR, not by
   editing dashboards by hand.
