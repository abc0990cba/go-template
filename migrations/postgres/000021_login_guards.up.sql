-- Progressive lockout: per-user password-failure counter and the pinned
-- lockout deadline. One row per user who ever failed a password check.

CREATE TABLE login_guards (
  user_id text PRIMARY KEY,
  failed_attempts int NOT NULL DEFAULT 0,
  locked_until timestamptz NOT NULL DEFAULT to_timestamp(0),
  updated_at timestamptz NOT NULL DEFAULT now()
);
