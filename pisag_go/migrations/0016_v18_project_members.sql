-- migrations/0016_v18_project_members.sql
-- v18: Context Expansion (minimal RBAC) - project_members hardening
-- 既存の public.project_members がある前提で「不足分だけ」追加/固定する
-- owner=ak で実行する想定

BEGIN;

-- 0) project_members が無い環境向け：最低限CREATE（あなたの環境では既にあるはず）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema='public' AND table_name='project_members'
  ) THEN
    CREATE TABLE public.project_members (
      id bigserial PRIMARY KEY,
      project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
      actor_type varchar(16) NOT NULL,   -- system|user|service
      actor_id   varchar(128) NOT NULL,  -- user_id / service_id etc
      role       varchar(32) NOT NULL,   -- viewer|operator|approver|admin
      status     varchar(16) NOT NULL DEFAULT 'active', -- active|disabled
      created_at timestamptz NOT NULL DEFAULT now(),
      updated_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE INDEX idx_project_members_project ON public.project_members(project_id, created_at DESC);
    CREATE INDEX idx_project_members_actor   ON public.project_members(actor_type, actor_id);
  END IF;
END$$;

-- 1) カラムの最低限（存在していなければ追加）
ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS actor_type varchar(16);

ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS actor_id varchar(128);

ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS role varchar(32);

ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS status varchar(16) NOT NULL DEFAULT 'active';

ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE public.project_members
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

-- 2) NOT NULL を強制（既存NULLがあれば先に埋める）
UPDATE public.project_members
SET status='active'
WHERE status IS NULL OR btrim(status)='';

UPDATE public.project_members
SET actor_type='user'
WHERE actor_type IS NULL OR btrim(actor_type)='';

UPDATE public.project_members
SET actor_id='unknown'
WHERE actor_id IS NULL OR btrim(actor_id)='';

UPDATE public.project_members
SET role='viewer'
WHERE role IS NULL OR btrim(role)='';

ALTER TABLE public.project_members
  ALTER COLUMN actor_type SET NOT NULL;

ALTER TABLE public.project_members
  ALTER COLUMN actor_id SET NOT NULL;

ALTER TABLE public.project_members
  ALTER COLUMN role SET NOT NULL;

-- 3) 最小のCHECK制約（空文字禁止 + 値域の固定）
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='project_members_status_nonempty') THEN
    ALTER TABLE public.project_members
      ADD CONSTRAINT project_members_status_nonempty CHECK (btrim(status) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='project_members_actor_type_ck') THEN
    ALTER TABLE public.project_members
      ADD CONSTRAINT project_members_actor_type_ck
        CHECK (actor_type IN ('system','user','service'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='project_members_role_ck') THEN
    ALTER TABLE public.project_members
      ADD CONSTRAINT project_members_role_ck
        CHECK (role IN ('viewer','operator','approver','admin'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='project_members_status_ck') THEN
    ALTER TABLE public.project_members
      ADD CONSTRAINT project_members_status_ck
        CHECK (status IN ('active','disabled'));
  END IF;
END$$;

-- 4) UNIQUE（同一actorが同一projectで同一roleを重複保持しない）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname='public' AND indexname='ux_project_members_actor_role'
  ) THEN
    CREATE UNIQUE INDEX ux_project_members_actor_role
      ON public.project_members(project_id, actor_type, actor_id, role);
  END IF;
END$$;

-- 5) updated_at trigger（projects と同じ set_updated_at を再利用）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_project_members_updated_at') THEN
      CREATE TRIGGER trg_project_members_updated_at
      BEFORE UPDATE ON public.project_members
      FOR EACH ROW
      EXECUTE FUNCTION public.set_updated_at();
    END IF;
  END IF;
END$$;

-- 6) インデックス（検索用）
CREATE INDEX IF NOT EXISTS idx_project_members_project_role
  ON public.project_members(project_id, role, status);

CREATE INDEX IF NOT EXISTS idx_project_members_actor_status
  ON public.project_members(actor_type, actor_id, status);

COMMIT;