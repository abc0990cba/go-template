-- Tickets: request types with approval chains, the tickets themselves, and
-- their approval steps, comments, and event history.

CREATE TABLE ticket_types (
  id text PRIMARY KEY,
  org_id text NOT NULL DEFAULT '',
  key text NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  steps jsonb NOT NULL DEFAULT '[]',
  active boolean NOT NULL DEFAULT true,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
-- (org_id, key) unique: '' is the platform-wide namespace.
CREATE UNIQUE INDEX uq_ticket_types_org_key ON ticket_types (org_id, key);

CREATE TABLE tickets (
  id text PRIMARY KEY,
  number int NOT NULL,
  org_id text NOT NULL DEFAULT '',
  type_id text NOT NULL REFERENCES ticket_types(id),
  type_key text NOT NULL DEFAULT '',
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  priority text NOT NULL DEFAULT 'medium',
  status text NOT NULL DEFAULT 'draft',
  current_step int NOT NULL DEFAULT 0,
  created_by text NOT NULL DEFAULT '',
  decided_by text NOT NULL DEFAULT '',
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tickets_org_status ON tickets (org_id, status, created_at DESC);
CREATE INDEX idx_tickets_created_by ON tickets (created_by, created_at DESC);
CREATE UNIQUE INDEX uq_tickets_org_number ON tickets (org_id, number);

CREATE TABLE ticket_approvals (
  id text PRIMARY KEY,
  ticket_id text NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  step_index int NOT NULL,
  name text NOT NULL DEFAULT '',
  approver_role text NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  decided_by text NOT NULL DEFAULT '',
  decided_at timestamptz,
  comment text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_ticket_approvals_step ON ticket_approvals (ticket_id, step_index);
CREATE INDEX idx_ticket_approvals_pending ON ticket_approvals (approver_role, status);

CREATE TABLE ticket_comments (
  id text PRIMARY KEY,
  ticket_id text NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  author_id text NOT NULL DEFAULT '',
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_comments_ticket ON ticket_comments (ticket_id, created_at);

CREATE TABLE ticket_events (
  id text PRIMARY KEY,
  ticket_id text NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  actor_id text NOT NULL DEFAULT '',
  action text NOT NULL,
  detail text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_events_ticket ON ticket_events (ticket_id, created_at);
