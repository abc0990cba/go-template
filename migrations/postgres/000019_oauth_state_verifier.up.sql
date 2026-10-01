-- PKCE (Twitter OAuth2 requires it) stores the code verifier with the
-- single-use state; empty for providers that do not use PKCE.
ALTER TABLE oauth_states ADD COLUMN code_verifier text NOT NULL DEFAULT '';
