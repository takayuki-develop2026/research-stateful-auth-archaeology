BEGIN;

CREATE TABLE IF NOT EXISTS public.privilege_grants_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  subject_type text NOT NULL CHECK (subject_type IN ('user','service')),
  subject_id text NOT NULL,

  granted_role text NOT NULL,
  scope_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  grant_reason_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  granted_by_user_id text NOT NULL,
  granted_at_utc timestamptz NOT NULL DEFAULT now(),

  revoked_at_utc timestamptz NULL,
  revoked_by_user_id text NULL,
  revoke_reason_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  CONSTRAINT pg_v21_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT pg_v21_subject_nonempty CHECK (btrim(subject_id) <> ''),
  CONSTRAINT pg_v21_role_nonempty CHECK (btrim(granted_role) <> ''),
  CONSTRAINT pg_v21_granted_by_nonempty CHECK (btrim(granted_by_user_id) <> '')
);

CREATE INDEX IF NOT EXISTS idx_privilege_grants_v21_project_subject_time
  ON public.privilege_grants_v21(project_id, subject_type, subject_id, granted_at_utc DESC);

COMMIT;