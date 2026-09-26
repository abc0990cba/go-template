-- Impersonation: which admin started the session (empty for normal logins).

ALTER TABLE sessions ADD COLUMN impersonated_by text NOT NULL DEFAULT '';
