-- Outbound transactional email: one row per message, whatever carried it
-- (inline send or the queue). The delivery log makes "did the reset mail
-- go out?" answerable and gives admins a retry button.

CREATE TABLE email_messages (
  id text PRIMARY KEY,
  to_address text NOT NULL,
  subject text NOT NULL,
  body text NOT NULL,
  kind text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'queued',
  attempts int NOT NULL DEFAULT 0,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  sent_at timestamptz
);

CREATE INDEX idx_email_messages_status ON email_messages(status, created_at DESC);
