# 10 — Flows: Event Streams

**In one sentence:** two external systems (a Kafka topic and a NATS
subject) stream user facts that are merged into one Postgres row with
**exactly-once** semantics — inbox dedupe + per-source sequence guards +
field ownership — and every applied merge publishes a confirmation event
through a **transactional outbox**, with a dead-letter queue and an admin
replay console for the failures.

This is the most instructive flow in the template: it demonstrates, on a
readable scale, the patterns that keep real event-driven systems correct.

## 10.1 The topology

```mermaid
flowchart LR
    subgraph producers["External producers"]
        K1["CRM / HR system"]
        N1["Contact service"]
    end
    subgraph brokers["Transports"]
        K[("Kafka topic<br/>user-profile.v1<br/>group go-template-enricher")]
        N[("NATS JetStream stream USERS<br/>subject users.contact.v1<br/>durable enricher-contact")]
        KO[("Kafka topic<br/>user-events.v1")]
        NO[("NATS subject<br/>users.events.v1")]
    end
    subgraph worker["worker process"]
        KC["Kafka consumer<br/>manual offsets"]
        NC["NATS pull consumer<br/>AckExplicit, AckWait 30s"]
        APPLY["streams.ApplyStreamEvent<br/>one transaction per event"]
        RELAY["Outbox relay<br/>1s tick, batch 100, lease 60s"]
        DLQ["Dead letters"]
    end
    PG[("Postgres<br/>users row + processed_events<br/>+ outbox")]
    ADMIN["Admin console<br/>Streams page: outbox + DLQ + replay"]

    K1 --> K
    N1 --> N
    K --> KC --> APPLY
    N --> NC --> APPLY
    APPLY --> PG
    APPLY -->|INSERT outbox| PG
    PG --> RELAY
    RELAY --> KO
    RELAY --> NO
    APPLY -->|malformed / unknown_user / max_attempts| DLQ
    ADMIN -->|replay through live handler| DLQ
```

Message envelope (both transports): `{"event_id", "external_ref", "seq",
"occurred_at", "data": {…}}` — `external_ref` joins to
`users.external_ref`; `seq` is the producer's per-source monotonic
counter.

## 10.2 Exactly-once, in one transaction

```mermaid
sequenceDiagram
    autonumber
    participant C as Consumer (kafka|nats)
    participant S as StreamsService
    participant P as Postgres (one TX)
    C->>S: HandleProfileEvent(envelope)
    S->>S: validate envelope — malformed → PermanentError → park in DLQ
    S->>S: retry 3 × 200 ms for transient store errors
    S->>P: SELECT user BY external_ref FOR UPDATE
    Note over P: serializes ALL sources for this user
    alt user missing after retries
        S->>P: INSERT dead_letters (unknown_user) — consumer commits past it
    else user found
        S->>P: INSERT processed_events (source, event_id) ON CONFLICT DO NOTHING
        alt 0 rows inserted
            Note over S: status = deduped (redelivery) — ack, done
        else inserted
            S->>P: UPDATE users SET enrich_profile = enrich_profile || payload,<br/>enrich_profile_seq = seq WHERE seq < incoming
            alt 0 rows (older seq)
                Note over S: status = stale — recorded, row untouched
            else applied
                S->>P: INSERT outbox (user.enriched event, trace headers)
            end
        end
    end
    C->>C: commit Kafka offset / ack NATS — only after TX result
```

The three guards and what each kills:

| Problem | Guard | Outcome |
|---|---|---|
| Broker redelivery (at-least-once) | `processed_events` inbox, PK `(source, event_id)` | `deduped` — no double apply |
| Out-of-order delivery within a source | per-source seq on the user row (`WHERE enrich_profile_seq < $seq`) | `stale` — old event ignored forever |
| Cross-source interference | **field ownership**: Kafka owns `enrich_profile` (company, job_title, enriched_by); NATS owns `enrich_contact` (phone, preferred_channel, enriched_by) — JSONB merge is per-source column | sources can never overwrite each other |

Kafka commits offsets only after the transaction wins
(`DisableAutoCommit` + `BlockRebalanceOnPoll`); NATS acks with
`AckExplicit`, nacks transient errors with delay (`NakWithDelay`), and
acks parked (terminal) messages so poison never blocks the partition.
Kafka-side transient failures stop the batch uncommitted — backpressure —
with backoff 500 ms → 10 s, under a supervisor restarting consumers with
exponential backoff 2 s → 30 s.

## 10.3 The transactional outbox

Why: you cannot atomically "update Postgres and publish to Kafka" —
either the publish is lost (update committed, no event) or phantom (event
published, update rolled back). The outbox makes the DB the truth:

1. The apply transaction inserts the outbound event into `outbox` —
   commit is atomic with the merge.
2. The relay loop (worker, every 1 s) claims a batch with
   `FOR UPDATE SKIP LOCKED` + a 60 s lease (multi-worker safe), publishes
   each event to **both** brokers (Kafka keyed by aggregate; NATS with
   `Nats-Msg-Id` = outbox id for server-side dedupe), guarded by breakers
   `kafka.produce` / `nats.publish`.
3. Success → `processed_at`; failure → attempts+1, and at
   `APP_OUTBOX_MAX_ATTEMPTS` (10) the event parks in the DLQ as
   `max_attempts`.
4. Processed rows are deleted after 7 days.

Downstream consumers see `user.enriched` with payload
`{user_id, external_ref, source, seq}` and propagated trace headers.

## 10.4 Dead letters and the admin replay console

```mermaid
flowchart TB
    IN["incoming event"] --> V{"validate envelope?"}
    V -->|no| PARK1["DLQ: reason=malformed"]
    V -->|yes| U{"user exists<br/>after 3 retries?"}
    U -->|no| PARK2["DLQ: reason=unknown_user"]
    U -->|yes| APPLY["apply (§10.2)"]
    OUTBOX["outbox publish"] -->|attempts exhausted| PARK3["DLQ: reason=max_attempts"]
    PARK1 & PARK2 & PARK3 --> TABLE[("dead_letters")]
    TABLE --> LIST["GET /api/streams/dead-letters<br/>(streams:read)"]
    LIST --> RP["POST /api/streams/dead-letters/:id/replay<br/>(streams:replay)"]
    RP --> LIVE["re-run through the live handler"]
    RP --> AUD["audit streams.replayed / replay_failed"]
```

Replay goes through the *live* handler (fresh dedupe/seq checks), so a
fixed producer or a late-created user makes the same payload succeed.
Already-replayed letters answer `409 already replayed`. The console lives
at Admin → Streams alongside the outbox view (`GET /api/streams/outbox`
with `pending` counts).

## 10.5 Local playground (10 minutes, fully scripted)

```bash
make demo-up          # compose + Redpanda + NATS overlay
make demo-enrichment  # publish both sources for demo-u-0001, poll until merged
make demo-dlq         # poison (malformed) + ghost user → both letters, then replay
make demo-outbox      # consume user-events.v1 and print relayed output
make demo-breaker     # in-process circuit-breaker state transitions
make demo-ratelimit   # 200 concurrent hits → 429s + Retry-After
make demo-idempotency # duplicate ticket POST with one key → one ticket
```

`cmd/devtools` subcommands (`kafka-profile`, `nats-contact`, `demo`,
`demo-dlq`, `dlq-list`, `dlq-replay`, `consume-user-events`, `hammer`,
`flaky-server`) drive each scenario; the UI equivalents are Admin →
Streams and the per-user "Publish demo enrichment" action.

## 10.6 Metrics to watch in production

| Metric | Meaning |
|---|---|
| `app_enrichment_events_total{source,result}` | applied/deduped/stale/parked per source |
| `app_outbox_pending` / `app_outbox_relay_total{result}` | relay lag and failures |
| `app_dead_letters_pending` / `app_dead_letters_replayed_total` | backlog and recovery |
| `app_cb_state{name}` | breaker states for `enrichment.http`, `kafka.produce`, `nats.publish`, `webhooks.http` |

Alert-worthy conditions: rising `app_outbox_pending` (relay behind),
sustained `app_dead_letters_pending` growth (producer or data contract
broken), any breaker sitting at `open` (2).

Next: [11 — Security](11-security.md).
