# ADR 0011 — Exactly-Once Stream Consumption: Inbox + Per-Source Sequences + Field Ownership

**Status:** Accepted

## Context

Two external systems stream user facts over Kafka and NATS. Brokers
guarantee at-least-once delivery; redeliveries, rebalances and
out-of-order sends are normal. Naïve consumption duplicates or resurrects
old data; "just use Kafka transactions/idempotent producer" solves
producing, not the consume-apply side.

## Decision

Three cooperating guards inside **one Postgres transaction** per event:

1. **Inbox** — `processed_events(source, event_id)` PK with
   `INSERT … ON CONFLICT DO NOTHING`; zero rows inserted means redelivery
   → status `deduped`, nothing applied.
2. **Per-source sequence guard** — each source carries a monotonic `seq`;
   the apply is `UPDATE … SET enrich_X = …, enrich_X_seq = $seq WHERE
   enrich_X_seq < $seq`; zero rows updated means an older event arrived
   late → status `stale`, recorded but ignored forever.
3. **Field ownership** — each source owns exactly one JSONB column on the
   user (`kafka → enrich_profile`, `nats → enrich_contact`); merge is a
   per-column overlay, so cross-source events can never overwrite each
   other.

Broker acknowledgment (Kafka offset commit with `DisableAutoCommit` +
`BlockRebalanceOnPoll`; NATS `AckExplicit`) happens **only after** the
transaction's outcome is known. Terminal failures (malformed envelope,
unknown user after 3 retries) park in the dead-letter queue and are
committed past; transient failures nack/stop-uncommitted (backpressure).

## Consequences

**Positive:** correct under every delivery pathology the brokers can
produce; auditable outcomes (`applied/deduped/stale` as a metric
dimension); the same pattern transfers to any at-least-once source.

**Negative:** every event costs a transaction (fine at template scale);
requires producers to send monotonic `seq` per source — a data-contract
requirement, documented in the envelope spec; `processed_events` grows
unboundedly (prune job is a listed extension; the table is tiny rows).

## Alternatives considered

- *Kafka transactions / EOS* — covers Kafka→Kafka pipelines, not the
  DB-apply side; also couples us to one broker's semantics.
- *Idempotent consumer by natural key only* — cannot reject stale
  ordering without the seq.
- *External dedupe store (Redis)* — a second truth that can disagree with
  Postgres under failure; the inbox lives where the data lives.
