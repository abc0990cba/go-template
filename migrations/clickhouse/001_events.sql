CREATE TABLE IF NOT EXISTS analytics_events (
  occurred_at DateTime64(3),
  event_name LowCardinality(String),
  user_id String,
  request_id String,
  properties String
) ENGINE = MergeTree
ORDER BY (event_name, occurred_at);
