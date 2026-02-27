-- migrations/0049_v84_dedupe_group_members.sql
-- v8.4: dedupe_group_members (membership list; idempotent by UNIQUE)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.dedupe_group_members (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  group_id bigint NOT NULL REFERENCES public.dedupe_groups(id) ON DELETE CASCADE,
  candidate_id bigint NOT NULL REFERENCES public.discovery_candidates(id) ON DELETE CASCADE,

  member_role varchar(16) NOT NULL DEFAULT 'member', -- member|suspect
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dedupe_group_members_role_ck CHECK (lower(member_role) IN ('member','suspect'))
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_dedupe_group_member_v84') THEN
    ALTER TABLE public.dedupe_group_members
      ADD CONSTRAINT ux_dedupe_group_member_v84 UNIQUE (group_id, candidate_id);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_dedupe_group_members_v84_project_group
  ON public.dedupe_group_members(project_id, group_id);

CREATE INDEX IF NOT EXISTS idx_dedupe_group_members_v84_project_candidate
  ON public.dedupe_group_members(project_id, candidate_id);

COMMIT;