-- migrations/0050_v84_dedupe_decisions.sql
-- v8.4: dedupe_decisions (decision ledger; keep payload small; details -> evidence_assets)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.dedupe_decisions (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  group_id bigint NOT NULL REFERENCES public.dedupe_groups(id) ON DELETE CASCADE,

  decided_by_type varchar(16) NOT NULL, -- system|human|service
  decided_by varchar(128) NULL,

  decision_type varchar(32) NOT NULL, -- propose_group|propose_winner|confirm_winner|merge_fields|reject_candidate|reject_all
  decision_payload jsonb NOT NULL DEFAULT '{}'::jsonb, -- SMALL only (rules/ids). NO bodies.
  decision_evidence_ref uuid NULL, -- attach rationale/details via evidence_assets

  trace_id varchar(128) NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  decided_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dedupe_decisions_by_type_ck CHECK (lower(decided_by_type) IN ('system','human','service')),
  CONSTRAINT dedupe_decisions_type_ck CHECK (lower(decision_type) IN (
    'propose_group','propose_winner','confirm_winner','merge_fields','reject_candidate','reject_all'
  )),
  CONSTRAINT dedupe_decisions_trace_nonempty CHECK (btrim(trace_id) <> '')
);

CREATE INDEX IF NOT EXISTS idx_dedupe_decisions_v84_project_group_time
  ON public.dedupe_decisions(project_id, group_id, decided_at DESC);

CREATE INDEX IF NOT EXISTS idx_dedupe_decisions_v84_trace
  ON public.dedupe_decisions(trace_id);

COMMIT;