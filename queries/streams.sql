-- Stream events, transactional outbox, dead letters, and API idempotency
-- keys. The apply queries run inside one store-managed transaction: the
-- inbox insert dedupes, the seq guard drops stale events, the JSONB merge
-- touches only the fields the source owns.

-- name: InsertProcessedEvent :execrows
INSERT INTO processed_events (source, event_id, user_id, seq)
VALUES ($1, $2, $3, $4)
ON CONFLICT (source, event_id) DO NOTHING;

-- name: ApplyUserProfile :execrows
UPDATE users
SET enrich_profile = enrich_profile || $2::jsonb,
    enrich_profile_seq = $3,
    enriched_at = now(),
    updated_at = now()
WHERE id = $1 AND enrich_profile_seq < $3;

-- name: ApplyUserContact :execrows
UPDATE users
SET enrich_contact = enrich_contact || $2::jsonb,
    enrich_contact_seq = $3,
    enriched_at = now(),
    updated_at = now()
WHERE id = $1 AND enrich_contact_seq < $3;

-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, event_type, aggregate_id, payload, headers)
VALUES ($1, $2, $3, $4::jsonb, $5::jsonb);

-- name: ClaimOutboxBatch :many
UPDATE outbox SET claimed_at = now()
WHERE id IN (
  SELECT o.id FROM outbox o
  WHERE o.processed_at IS NULL
    AND (o.claimed_at IS NULL OR o.claimed_at < now() - make_interval(secs => sqlc.arg('lease_secs')::int))
    AND o.attempts < sqlc.arg('max_attempts')
  ORDER BY o.created_at
  LIMIT sqlc.arg('lim')
)
RETURNING id, event_type, aggregate_id, payload, headers, attempts, claimed_at, processed_at, created_at;

-- name: MarkOutboxProcessed :execrows
UPDATE outbox SET processed_at = now() WHERE id = ANY(sqlc.arg('ids')::text[]);

-- name: ReleaseOutboxBatch :exec
UPDATE outbox SET claimed_at = NULL, attempts = attempts + 1
WHERE id = ANY(sqlc.arg('ids')::text[]);

-- name: FailOutboxPermanently :exec
UPDATE outbox SET processed_at = now(), claimed_at = NULL
WHERE id = ANY(sqlc.arg('ids')::text[]);

-- name: CountOutboxPending :one
SELECT count(*) FROM outbox WHERE processed_at IS NULL;

-- name: ListOutboxOffset :many
SELECT id, event_type, aggregate_id, payload, headers, attempts, claimed_at, processed_at, created_at
FROM outbox
WHERE (sqlc.arg('pending')::bool = false OR processed_at IS NULL)
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: DeleteOldOutbox :execrows
-- Retention for already-published events; a week is plenty for debugging.
DELETE FROM outbox WHERE processed_at IS NOT NULL AND processed_at < now() - interval '7 days';

-- name: InsertDeadLetter :exec
INSERT INTO dead_letters (id, source, topic, payload, reason, attempts)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO NOTHING;

-- name: GetDeadLetter :one
SELECT id, source, topic, payload, reason, attempts, replayed_at, created_at
FROM dead_letters
WHERE id = $1;

-- name: ListDeadLettersOffset :many
SELECT id, source, topic, payload, reason, attempts, replayed_at, created_at
FROM dead_letters
WHERE (sqlc.arg('pending')::bool = false OR replayed_at IS NULL)
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: MarkDeadLetterReplayed :execrows
UPDATE dead_letters SET replayed_at = now() WHERE id = $1 AND replayed_at IS NULL;

-- name: CountDeadLettersPending :one
SELECT count(*) FROM dead_letters WHERE replayed_at IS NULL;

-- name: ReserveIdempotencyKey :one
INSERT INTO idempotency_keys (key, user_id, method, path, request_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, key) DO NOTHING
RETURNING key, user_id, method, path, request_hash, status, response_body, content_type, created_at, expires_at;

-- name: GetIdempotencyKey :one
SELECT key, user_id, method, path, request_hash, status, response_body, content_type, created_at, expires_at
FROM idempotency_keys
WHERE user_id = $1 AND key = $2;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET status = $3, response_body = $4, content_type = $5
WHERE user_id = $1 AND key = $2 AND status = 0;

-- name: PruneIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < now();

-- name: DeleteDemoProcessedEvents :execrows
DELETE FROM processed_events WHERE user_id LIKE 'demo-%';

-- name: DeleteDemoOutbox :execrows
DELETE FROM outbox WHERE aggregate_id LIKE 'demo-%';

-- name: GetUserEnrichSeqs :one
SELECT enrich_profile_seq, enrich_contact_seq
FROM users
WHERE id = $1;
