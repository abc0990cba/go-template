-- Streams: user enrichment from Kafka + NATS, transactional outbox,
-- dead letters, and API idempotency keys.

ALTER TABLE users
  ADD COLUMN external_ref text,
  ADD COLUMN enrich_profile jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN enrich_contact jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN enrich_profile_seq bigint NOT NULL DEFAULT 0,
  ADD COLUMN enrich_contact_seq bigint NOT NULL DEFAULT 0,
  ADD COLUMN enriched_at timestamptz;

CREATE UNIQUE INDEX users_external_ref_key ON users (external_ref) WHERE external_ref IS NOT NULL;

-- Inbox: one row per applied stream event. The (source, event_id) primary
-- key turns at-least-once broker delivery into exactly-once application.
CREATE TABLE processed_events (
  source text NOT NULL,
  event_id text NOT NULL,
  user_id text NOT NULL,
  seq bigint NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (source, event_id)
);

-- Outbox: domain events written in the same transaction as the change they
-- describe; the worker relay publishes them and marks them processed.
CREATE TABLE outbox (
  id text PRIMARY KEY,
  event_type text NOT NULL,
  aggregate_id text NOT NULL,
  payload jsonb NOT NULL,
  headers jsonb NOT NULL DEFAULT '{}'::jsonb,
  attempts int NOT NULL DEFAULT 0,
  claimed_at timestamptz,
  processed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_pending_idx ON outbox (created_at) WHERE processed_at IS NULL;

-- Dead letters: poison or unprocessable messages parked for inspection and
-- replay instead of blocking their stream.
CREATE TABLE dead_letters (
  id text PRIMARY KEY,
  source text NOT NULL,
  topic text NOT NULL DEFAULT '',
  payload text NOT NULL,
  reason text NOT NULL,
  attempts int NOT NULL DEFAULT 1,
  replayed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX dead_letters_pending_idx ON dead_letters (created_at DESC) WHERE replayed_at IS NULL;

-- API idempotency keys: reserved request + stored response for replays.
CREATE TABLE idempotency_keys (
  key text NOT NULL,
  user_id text NOT NULL,
  method text NOT NULL,
  path text NOT NULL,
  request_hash text NOT NULL,
  status int NOT NULL DEFAULT 0,
  response_body bytea,
  content_type text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, key)
);

CREATE INDEX idempotency_keys_expiry_idx ON idempotency_keys (expires_at);
