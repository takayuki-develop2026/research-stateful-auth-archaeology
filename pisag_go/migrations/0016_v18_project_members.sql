BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'project_members'
  ) THEN
    CREATE TABLE public.project_members (
      id bigserial PRIMARY KEY,
      project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
      actor_type varchar(16) NOT NULL,
      actor_id   varchar(128) NOT NULL,
      role       varchar(32) NOT NULL,
      status     varchar(16) NOT NULL DEFAULT 'active',
      created_at timestamptz NOT NULL DEFAULT now(),
      updated_at timestamptz NOT NULL DEFAULT now()
    );
  END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_project_members_project_id
  ON public.project_members(project_id);

CREATE INDEX IF NOT EXISTS idx_project_members_actor
  ON public.project_members(actor_type, actor_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_project_members_project_actor_role
  ON public.project_members(project_id, actor_type, actor_id, role);

COMMIT;
