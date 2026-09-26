-- Audit trail: who did what to which resource, with the request context.
-- diff carries a {"before":…,"after":…} jsonb snapshot for mutations.

CREATE TABLE audit_log (
  id bigserial PRIMARY KEY,
  actor_id text NOT NULL DEFAULT '',
  actor_email text NOT NULL DEFAULT '',
  action text NOT NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id text NOT NULL DEFAULT '',
  domain text NOT NULL DEFAULT '',
  diff jsonb NOT NULL DEFAULT '{}',
  ip text NOT NULL DEFAULT '',
  request_id text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_created ON audit_log(created_at DESC, id DESC);
CREATE INDEX idx_audit_actor ON audit_log(actor_id, created_at DESC);
CREATE INDEX idx_audit_resource ON audit_log(resource_type, resource_id);
