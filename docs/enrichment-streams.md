# Enrichment Streams: Kafka + NATS

User data merges from two independent external systems into the same `users` rows:

| Stream | Transport | Topic / subject | Owns |
|---|---|---|---|
| Profile | Kafka (consumer group `go-template-enricher`) | `user-profile.v1` | `users.enrich_profile` (company, job_title, …) |
| Contact | NATS JetStream (durable `enricher-contact`) | `users.contact.v1` | `users.enrich_contact` (phone, …) |

Both run in the **worker** process and are optional per configuration: leave `APP_KAFKA_BROKERS` / `APP_NATS_URL` empty and that side is off, with a startup warning if it was configured but unreachable.

## The wire format

One JSON envelope for both streams; `data` is a raw JSON object of the fields that source owns:

```json
{
  "event_id": "unique-per-source-id",
  "external_ref": "demo-u-0001",
  "seq": 42,
  "occurred_at": "2026-09-28T12:00:00Z",
  "data": {"company": "Acme Corp", "job_title": "Engineer"}
}
```

- `external_ref` correlates the event with a user (unique column, settable on user create/update, seeded for every demo user).
- `seq` is the source's per-user monotonic sequence number.

## Why there are no race conditions

Every event applies in **one transaction** (`internal/platform/postgres/streams.go`):

1. **Row lock** — the user row is locked by `external_ref` (`SELECT … FOR UPDATE`), so all sources for one user serialize in one place.
2. **Inbox dedupe** — `INSERT INTO processed_events … ON CONFLICT DO NOTHING`; a conflict means the event already applied: ack and skip. At-least-once delivery from either broker becomes exactly-once application.
3. **Sequence guard** — the update runs `WHERE enrich_<source>_seq < $seq`; an old sequence arriving late is recorded as processed *without touching the row*.
4. **Field ownership** — each source merges only its own JSONB column (`enrich_profile || $payload`), so concurrent cross-broker events can never overwrite each other's fields.
5. **Atomic outbox** — an applied event commits its `user.enriched` outbox row in the same transaction. A crash can never leave the row changed without its event.

A concurrency test (`internal/service/streams_test.go`, `TestConcurrentSourcesNoLostUpdates`) hammers one user from both sources with duplicate deliveries and asserts every field of every event lands exactly once; the contract suite runs the same scenarios against real Postgres, and `test/integration/streams_e2e_test.go` drives real Kafka + NATS containers end to end.

## Where things run

- **Worker**: both consumers, the outbox relay, and the idempotency sweep. It exposes its own `/metrics` on `APP_METRICS_ADDR` (compose maps 19090) because stream and relay gauges live in this process.
- **API**: no consumers. Its readiness check pings the brokers through **ping-only** clients — a consumer-group client that never polls would join the group and hold partitions away from the worker (a classic franz-go footgun this template guards against).

## Broker resilience

- **Kafka**: offsets commit only after the apply transaction commits. A transient failure (say, the database blipping) blocks its partition briefly — the uncommitted record redelivers, which is exactly the backpressure you want. Terminal failures (malformed payload, unknown user after brief retries) are parked in dead letters and then committed past, so poison cannot loop forever.
- **NATS JetStream**: explicit ack after commit, `NakWithDelay` for transient errors, ack after parking terminal ones. The durable consumer survives restarts; redeliveries hit the inbox dedupe.
- **Rebalance safety**: commits happen per processed batch with rebalances held between poll and commit (`BlockRebalanceOnPoll`).

## Transactional outbox

`users.enriched` events go to the `outbox` table (same transaction as the merge). The worker's relay loop (`APP_OUTBOX_INTERVAL`, default 1s):

1. claims a leased batch (`FOR UPDATE SKIP LOCKED`, stale leases reclaim),
2. publishes each event through the `kafka.produce` / `nats.publish` circuit breakers — Kafka keyed by user id, NATS with `Nats-Msg-Id` so the stream itself deduplicates a crash between publish and mark,
3. marks published rows processed, or releases failures with an attempts bump; the final attempt dead-letters the event.

Read what the relay published with `devtools consume-user-events` (or `make demo-outbox`); inspect the queue in the app on **Admin → Streams** or `GET /api/streams/outbox`.

## Circuit breakers

`internal/reliability/circuitbreaker.go` — stdlib only. Consecutive failures trip it open (fail fast with `ErrCircuitOpen`), one probe after `APP_CB_OPEN_TIMEOUT` closes or re-opens it. Named breakers: `enrichment.http`, `webhooks.http`, `kafka.produce`, `nats.publish`. Metrics: `app_cb_state{name}` and `app_cb_transitions_total{name,from,to}`. Tune with `APP_CB_FAILURE_THRESHOLD`.

## Dead letters and replay

`dead_letters` is the single parking lot for malformed payloads, unknown users, and exhausted relay events. Inspect and replay via the admin API or the SPA's Streams page; every replay is audit-logged. `unknown_user` letters are the common case — the user row arrives late — so `devtools dlq-replay --id …` re-runs them through the live handler path once the user exists.

## Rate limiting

- `auth` scope (login/register/reset): tight per-IP budget (`APP_AUTH_RATE_RPS`).
- `api` scope (everything under `/api`): general budget (`APP_API_RATE_RPS`, default 50/s).

With Redis configured the budget is one shared sliding window across all api processes; without it, per-process token buckets. Limiter outages fail **open** — shedding traffic because the limiter is down would turn one outage into two — and count `app_rate_limit_errors_total`.

## API idempotency keys

POST routes can accept an `Idempotency-Key` header (wired on `POST /api/tickets`):

- first request: reserves the key (single `INSERT … ON CONFLICT`) and stores the response when done;
- identical retry: replays the stored response with `Idempotency-Replayed: true`;
- same key, different body: `422`;
- original still in flight: `409` with `Retry-After`.

Concurrency-safe by construction — the reservation itself is the database race. Stored responses expire after `APP_IDEMPOTENCY_TTL`; the worker prunes them every 10 minutes.

## Try it locally

**Fastest check — the admin console.** Open any user's detail page and click **Send test event**
on the "Profile (Kafka)" or "Contact (NATS)" card. It calls `POST /api/users/:id/enrich-demo`,
which publishes a real event into that stream; the worker applies it through the normal
exactly-once pipeline and the card refetches with the merged fields. Requires the user to have an
`external_ref` (all demo users do) and the brokers to be up. The users table rows are clickable
and every row icon explains itself on hover.

```sh
make demo-up                 # core stack + Redpanda + NATS
make demo-enrichment         # publish to both streams, poll the API, print the merged user
make demo-kafka-profile      # just the Kafka side → "Profile (Kafka)" card on the user page
make demo-nats-contact       # just the NATS side → "Contact (NATS)" card
make demo-dlq                # poison + unknown-user events → dead letters → replay
make demo-outbox             # watch the relay publish to user-events.v1
make demo-breaker            # circuit breaker open → half-open → closed, live
make demo-ratelimit          # 429s with Retry-After
make demo-idempotency        # retried POST replays instead of double-filing
```

All of these are thin wrappers over `cmd/devtools`, which also takes `--brokers/--url/--api` flags directly. Login credentials default to the demo account; see `.env.example`.
