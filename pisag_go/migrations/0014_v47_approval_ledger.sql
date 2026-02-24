-- migrations/0014_v47_approval_ledger.sql
-- v4.7: Approval Ledger (default-deny)
--
-- 목적:
-- - v4.6 の catalog_publish_commits(status=proposed) を「承認要求」に載せる
-- - 承認/却下の意思決定を台帳として永続化し、監査可能にする
-- - default-deny: approval が無い限り publish を confirmed にしない（UseCase側で強制）
--
-- 方針:
-- - approval_requests: 1 commit につき 0/1（当面）。将来は再申請のため version を足せる
-- - approval_decisions: 1 request につき複数（履歴）。最終状態は latest で判定
-- - idempotency: (project_id, commit_id) unique
--
-- 依存:
-- - public.catalog_publish_commits(commit_id uuid, project_id text, trace_id uuid, status text)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------
-- 1) approval_requests
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS public.approval_requests (
  request_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id    text NOT NULL,

  -- the publish commit being requested
  commit_id     uuid NOT NULL REFERENCES public.catalog_publish_commits(commit_id) ON DELETE CASCADE,

  -- traceability
  trace_id      uuid NOT NULL,

  -- state:
  -- pending  : waiting for decision
  -- approved : approved (publish can be confirmed)
  -- rejected : rejected (publish remains proposed or becomes failed by policy)
  status        text NOT NULL DEFAULT 'pending',

  -- who requested (system/user). v4.7 minimal: text
  requested_by_type text NOT NULL DEFAULT 'system', -- system/user
  requested_by_id   text NULL,

  reason        text NULL,

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- idempotency: one request per (project, commit) for now
CREATE UNIQUE INDEX IF NOT EXISTS approval_requests_project_commit_uniq
  ON public.approval_requests (project_id, commit_id);

CREATE INDEX IF NOT EXISTS approval_requests_project_created_idx
  ON public.approval_requests (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS approval_requests_commit_idx
  ON public.approval_requests (commit_id);

CREATE INDEX IF NOT EXISTS approval_requests_trace_idx
  ON public.approval_requests (trace_id, created_at DESC);

-- status constraint
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_requests_status_chk') THEN
    ALTER TABLE public.approval_requests
      ADD CONSTRAINT approval_requests_status_chk
      CHECK (status IN ('pending','approved','rejected'));
  END IF;
END$$;

-- requested_by_type constraint
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_requests_requested_by_type_chk') THEN
    ALTER TABLE public.approval_requests
      ADD CONSTRAINT approval_requests_requested_by_type_chk
      CHECK (requested_by_type IN ('system','user'));
  END IF;
END$$;

-- ------------------------------------------
-- 2) approval_decisions
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS public.approval_decisions (
  decision_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id    text NOT NULL,
  request_id    uuid NOT NULL REFERENCES public.approval_requests(request_id) ON DELETE CASCADE,

  trace_id      uuid NOT NULL,

  -- decision:
  -- approve / reject
  decision      text NOT NULL,

  -- who decided
  decided_by_type text NOT NULL DEFAULT 'user', -- system/user
  decided_by_id   text NULL,

  comment       text NULL,

  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS approval_decisions_project_created_idx
  ON public.approval_decisions (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS approval_decisions_request_created_idx
  ON public.approval_decisions (request_id, created_at DESC);

CREATE INDEX IF NOT EXISTS approval_decisions_trace_idx
  ON public.approval_decisions (trace_id, created_at DESC);

-- decision constraint
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_decisions_decision_chk') THEN
    ALTER TABLE public.approval_decisions
      ADD CONSTRAINT approval_decisions_decision_chk
      CHECK (decision IN ('approve','reject'));
  END IF;
END$$;

-- decided_by_type constraint
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_decisions_decided_by_type_chk') THEN
    ALTER TABLE public.approval_decisions
      ADD CONSTRAINT approval_decisions_decided_by_type_chk
      CHECK (decided_by_type IN ('system','user'));
  END IF;
END$$;

-- ------------------------------------------
-- 3) updated_at trigger
-- ------------------------------------------
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_approval_requests_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
      CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $fn$
      BEGIN
        NEW.updated_at := now();
        RETURN NEW;
      END;
      $fn$ LANGUAGE plpgsql;
    END IF;

    CREATE TRIGGER trg_approval_requests_updated_at
    BEFORE UPDATE ON public.approval_requests
    FOR EACH ROW
    EXECUTE FUNCTION public.set_updated_at();
  END IF;
END$$;

COMMIT;