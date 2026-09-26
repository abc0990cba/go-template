-- name: Analytics :one
SELECT
  (SELECT count(*) FROM users) AS total_users,
  (SELECT count(*) FROM auth_identities WHERE provider = 'password') AS password_identities,
  (SELECT count(*) FROM auth_identities WHERE provider = 'google') AS google_identities,
  (SELECT count(*) FROM auth_identities WHERE provider = 'github') AS github_identities,
  (SELECT count(*) FROM user_enrichment_profiles) AS enrichment_succeeded,
  (SELECT count(*) FROM jobs WHERE status = 'queued') AS jobs_queued,
  (SELECT count(*) FROM jobs WHERE status = 'done') AS jobs_done,
  (SELECT count(*) FROM jobs WHERE status = 'failed') AS jobs_failed;
