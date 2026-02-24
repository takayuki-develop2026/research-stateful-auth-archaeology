BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='audit_events_action_nonempty') THEN
    ALTER TABLE public.audit_events
      ADD CONSTRAINT audit_events_action_nonempty CHECK (btrim(action::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='audit_events_result_nonempty') THEN
    ALTER TABLE public.audit_events
      ADD CONSTRAINT audit_events_result_nonempty CHECK (btrim(result::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='audit_events_trace_nonempty') THEN
    ALTER TABLE public.audit_events
      ADD CONSTRAINT audit_events_trace_nonempty CHECK (btrim(trace_id::text) <> '');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='audit_events_result_domain_ck') THEN
    ALTER TABLE public.audit_events
      ADD CONSTRAINT audit_events_result_domain_ck
        CHECK (result IN ('ok','denied','failed'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='audit_events_actor_type_domain_ck') THEN
    ALTER TABLE public.audit_events
      ADD CONSTRAINT audit_events_actor_type_domain_ck
        CHECK (actor_type IN ('system','user','service'));
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_audit_events_project_result_time
  ON public.audit_events(project_id, result, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_project_actor_time
  ON public.audit_events(project_id, actor_type, actor_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_project_target_time
  ON public.audit_events(project_id, target_type, target_id, created_at DESC);

COMMIT;
