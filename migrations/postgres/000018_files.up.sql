-- File metadata: blobs live in the file store (S3/MinIO or the in-process
-- dev store); this table owns the permissions-relevant facts.

CREATE TABLE files (
  id text PRIMARY KEY,
  owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id text NOT NULL DEFAULT '',
  key text NOT NULL UNIQUE,
  filename text NOT NULL,
  content_type text NOT NULL DEFAULT '',
  size_bytes bigint NOT NULL DEFAULT 0,
  checksum_sha256 text NOT NULL DEFAULT '',
  ref_type text NOT NULL DEFAULT '',
  ref_id text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE INDEX idx_files_owner ON files(owner_id, created_at DESC);
CREATE INDEX idx_files_ref ON files(ref_type, ref_id) WHERE deleted_at IS NULL;
