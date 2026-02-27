-- migrations/0058_v50_routing_metrics_daily.sql
-- v5.0: routing_metrics_daily (observations for scoring; v9/v10 will extend)
-- Evidence alignment: snapshot_evidence_ref (uuid) -> evidence_assets(project_id,evidence_ref)
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.routing_metrics_daily (
  id                  bigserial PRIMARY KEY,
  project_id           varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  metric_date          date NOT NULL,

  provider_id          uuid NOT NULL REFERENCES public.providers(provider_id) ON DELETE RESTRICT,
  route_id             uuid NOT NULL REFERENCES public.provider_routes(route_id) ON DELETE RESTRICT,

  success_rate         numeric NOT NULL DEFAULT 0.0,  -- 0..1
  p95_latency_ms       int NOT NULL DEFAULT 0,
  avg_cost_minor       bigint NOT NULL DEFAULT 0,
  sample_n             int NOT NULL DEFAULT 0,

  snapshot_evidence_ref uuid NOT NULL,

  created_at           timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='routing_metrics_success_ck') THEN
    ALTER TABLE public.routing_metrics_daily
      ADD CONSTRAINT routing_metrics_success_ck CHECK (success_rate >= 0.0 AND success_rate <= 1.0);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_routing_metrics_daily') THEN
    ALTER TABLE public.routing_metrics_daily
      ADD CONSTRAINT ux_routing_metrics_daily UNIQUE (project_id, metric_date, provider_id, route_id);
  END IF;
END$$;

-- FK to evidence_assets by (project_id, evidence_ref)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='routing_metrics_snapshot_fk') THEN
    ALTER TABLE public.routing_metrics_daily
      ADD CONSTRAINT routing_metrics_snapshot_fk
      FOREIGN KEY (project_id, snapshot_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_routing_metrics_project_date
  ON public.routing_metrics_daily(project_id, metric_date DESC);

CREATE INDEX IF NOT EXISTS idx_routing_metrics_project_provider
  ON public.routing_metrics_daily(project_id, provider_id, metric_date DESC);

COMMIT;