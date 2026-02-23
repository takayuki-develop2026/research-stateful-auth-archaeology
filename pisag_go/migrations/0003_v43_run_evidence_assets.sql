CREATE TABLE IF NOT EXISTS run_evidence_assets (
  id bigserial PRIMARY KEY,
  run_id text NOT NULL,
  trace_id text NOT NULL,
  kind text NOT NULL, -- e.g. "fetch_body"
  content_type text,
  byte_size int NOT NULL,
  sha256 text NOT NULL,
  final_url text NOT NULL,
  stored_path text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS run_evidence_assets_uniq
  ON run_evidence_assets (run_id, kind, sha256);

CREATE INDEX IF NOT EXISTS run_evidence_assets_run_idx
  ON run_evidence_assets (run_id, created_at);