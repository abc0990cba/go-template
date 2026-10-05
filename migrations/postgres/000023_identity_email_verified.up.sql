-- Capture the provider's email_verified claim on linked identities
-- (defense-in-depth for any future OAuth login path; linking today already
-- requires a live session).

ALTER TABLE auth_identities ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
