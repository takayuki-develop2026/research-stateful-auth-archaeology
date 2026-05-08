BEGIN;

INSERT INTO public.projects (project_id, status, created_at, updated_at)
VALUES ('akproj_0000000000000000000', 'active', now(), now())
ON CONFLICT (project_id) DO UPDATE
SET
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO public.project_members (project_id, actor_type, actor_id, role, status, created_at, updated_at)
VALUES ('akproj_0000000000000000000', 'user', 'admin', 'admin', 'active', now(), now())
ON CONFLICT (project_id, actor_type, actor_id, role) DO UPDATE
SET
  status = EXCLUDED.status,
  updated_at = now();

COMMIT;
