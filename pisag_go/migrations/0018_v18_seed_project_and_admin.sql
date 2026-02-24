-- migrations/0018_v18_seed_project_and_admin.sql
-- v18: seed 1 project + 1 admin member (YOU-only)
-- Requirements:
-- - projects.id is varchar(26)
-- - project_members has UNIQUE (project_id, principal_type, principal_id)
-- - project_members also has UNIQUE (project_id, actor_type, actor_id, role) but actor_id can be chosen to avoid conflicts

BEGIN;

\set ON_ERROR_STOP on
\set project_id :AK_PROJECT_ID
\set project_name :AK_PROJECT_NAME
\set admin_user_id :AK_ADMIN_USER_ID

-- 1) project
INSERT INTO public.projects (id, name, status, created_at, updated_at)
VALUES (:'project_id', :'project_name', 'active', now(), now())
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

-- 2) member (admin)
-- actor_id は UNIQUE(project_id, actor_type, actor_id, role) を踏まえ、admin_user_id を混ぜて衝突しない値にする
INSERT INTO public.project_members
  (project_id, principal_type, principal_id, role, status, actor_type, actor_id, created_at, updated_at)
VALUES
  (:'project_id', 'user', :'admin_user_id', 'admin', 'active', 'system', ('seed:' || :'admin_user_id'), now(), now())
ON CONFLICT (project_id, principal_type, principal_id) DO UPDATE
SET role='admin',
    status='active',
    updated_at=now();

COMMIT;
