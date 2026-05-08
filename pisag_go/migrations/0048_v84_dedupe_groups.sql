-- migrations/0048_v84_dedupe_groups.sql
-- v8.4: dedupe_groups (group container; non-growing by UNIQUE)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.dedupe_groups (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  candidate_type varchar(32) NOT NULL, -- provider|provider_route|fee_model|catalog_source
  dedupe_key varchar(64) NOT NULL,

  status varchar(24) NOT NULL DEFAULT 'open', -- open|review_required|resolved
  winner_candidate_id bigint NULL, -- set when resolved

  resolution_type varchar(24) NULL, -- choose_winner|merge_fields|reject_all
  resolution_note_evidence_ref uuid NULL, -- store note as evidence if needed

  trace_id varchar(128) NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dedupe_groups_key_len CHECK (length(dedupe_key)=64),
  CONSTRAINT dedupe_groups_status_ck CHECK (lower(status) IN ('open','review_required','resolved')),
  CONSTRAINT dedupe_groups_resolution_ck CHECK (
    resolution_type IS NULL OR lower(resolution_type) IN ('choose_winner','merge_fields','reject_all')
  ),
  CONSTRAINT dedupe_groups_trace_nonempty CHECK (btrim(trace_id) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_dedupe_groups_v84') THEN
    ALTER TABLE public.dedupe_groups
      ADD CONSTRAINT ux_dedupe_groups_v84 UNIQUE (project_id, candidate_type, dedupe_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_dedupe_groups_v84_project_status_time
  ON public.dedupe_groups(project_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_dedupe_groups_v84_project_type_key
  ON public.dedupe_groups(project_id, candidate_type, dedupe_key);

-- winner_candidate_id is a soft link (avoid FK to allow reorder of migrations safely)
CREATE INDEX IF NOT EXISTS idx_dedupe_groups_v84_winner
  ON public.dedupe_groups(project_id, winner_candidate_id);

-- updated_at trigger
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_dedupe_groups_updated_at') THEN
      CREATE TRIGGER trg_dedupe_groups_updated_at
      BEFORE UPDATE ON public.dedupe_groups
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;