-- 0118_v15_funds_tables.sql
-- v15 Funds/Deadline SoT (P0)
-- evidence_refs: JSON array of evidence_ref (uuid string), NOT evidence_assets.id
-- run_id/trace_id/policy_version_id: text (aligned with your v14/v15 ops pattern)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================
-- disputes (SoT)
-- =========================
CREATE TABLE IF NOT EXISTS public.disputes_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  dispute_key text NOT NULL,          -- stripe:dp_... etc
  provider text NOT NULL,
  provider_dispute_id text NULL,
  provider_charge_id text NULL,
  provider_payment_intent_id text NULL,
  provider_object_refs jsonb NOT NULL DEFAULT '{}'::jsonb,

  shop_id text NULL,

  currency text NOT NULL,
  amount_minor bigint NOT NULL,
  reason_code text NULL,

  status text NOT NULL,               -- opened|evidence_required|under_review|won|lost|closed|review_required
  opened_at timestamptz NOT NULL,
  due_by timestamptz NULL,
  closed_at timestamptz NULL,
  resolution text NULL,

  source_event_key varchar(128) NULL, -- UTL event_key
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  meta jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT disputes_v15_status_ck CHECK (status IN ('opened','evidence_required','under_review','won','lost','closed','review_required')),
  CONSTRAINT disputes_v15_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT disputes_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array'),
  CONSTRAINT disputes_v15_provider_nonempty CHECK (btrim(provider) <> '')
);

ALTER TABLE public.disputes_v15
  ADD CONSTRAINT uq_disputes_v15_project_dispute_key UNIQUE (project_id, dispute_key);

CREATE INDEX IF NOT EXISTS idx_disputes_v15_project_status_due
  ON public.disputes_v15(project_id, status, due_by);

CREATE INDEX IF NOT EXISTS idx_disputes_v15_project_shop_opened
  ON public.disputes_v15(project_id, shop_id, opened_at DESC);

-- dispute events (idempotent by event_key)
CREATE TABLE IF NOT EXISTS public.dispute_events_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  dispute_id uuid NOT NULL REFERENCES public.disputes_v15(id) ON DELETE CASCADE,

  event_key varchar(128) NOT NULL,    -- UTL event_key
  event_type text NOT NULL,           -- opened|evidence_required|... etc
  occurred_at timestamptz NOT NULL,

  payload_evidence_ref uuid NULL,     -- evidence_ref UUID (NOT bigint)

  run_id text NOT NULL,
  trace_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dispute_events_v15_event_type_nonempty CHECK (btrim(event_type) <> '')
);

ALTER TABLE public.dispute_events_v15
  ADD CONSTRAINT uq_dispute_events_v15_dispute_event_key UNIQUE (dispute_id, event_key);

CREATE INDEX IF NOT EXISTS idx_dispute_events_v15_project_time
  ON public.dispute_events_v15(project_id, occurred_at DESC);

-- =========================
-- refunds (SoT + ledger link)
-- =========================
CREATE TABLE IF NOT EXISTS public.refunds_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  refund_key text NOT NULL,           -- stripe:re_... or internal:...
  provider text NOT NULL,
  provider_refund_id text NULL,
  provider_payment_ref text NULL,

  shop_id text NOT NULL,
  currency text NOT NULL,
  amount_minor bigint NOT NULL,

  status text NOT NULL,               -- requested|processing|succeeded|failed|retryable|review_required
  requested_at timestamptz NOT NULL,
  settled_at timestamptz NULL,
  failed_at timestamptz NULL,
  failure_code text NULL,
  failure_message text NULL,

  source_event_key varchar(128) NULL, -- UTL event_key
  posting_key char(64) NOT NULL,      -- ledger posting_key (idempotent)
  ledger_posting_id uuid NULL REFERENCES public.ledger_postings(id) ON DELETE SET NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  meta jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT refunds_v15_status_ck CHECK (status IN ('requested','processing','succeeded','failed','retryable','review_required')),
  CONSTRAINT refunds_v15_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT refunds_v15_provider_nonempty CHECK (btrim(provider) <> ''),
  CONSTRAINT refunds_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.refunds_v15
  ADD CONSTRAINT uq_refunds_v15_project_refund_key UNIQUE (project_id, refund_key);

ALTER TABLE public.refunds_v15
  ADD CONSTRAINT uq_refunds_v15_project_posting_key UNIQUE (project_id, posting_key);

CREATE INDEX IF NOT EXISTS idx_refunds_v15_project_status_time
  ON public.refunds_v15(project_id, status, requested_at DESC);

CREATE INDEX IF NOT EXISTS idx_refunds_v15_project_shop_time
  ON public.refunds_v15(project_id, shop_id, requested_at DESC);

-- =========================
-- holds (Funds state SoT)
-- =========================
CREATE TABLE IF NOT EXISTS public.holds_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  hold_key text NOT NULL,             -- internal:{...} or provider:{...}
  hold_type text NOT NULL,            -- dispute_hold|refund_hold|payout_hold|risk_hold|manual_hold

  scope_type text NOT NULL,           -- shop|user|platform|provider_account
  scope_id text NULL,

  currency text NOT NULL,
  amount_minor bigint NOT NULL,

  status text NOT NULL,               -- active|released|consumed|expired|review_required
  reason text NOT NULL,
  expires_at timestamptz NULL,

  source_event_key varchar(128) NULL,
  related_object_type text NULL,      -- dispute|refund|payout|settlement_batch
  related_object_key text NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  created_by text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT holds_v15_hold_type_ck CHECK (hold_type IN ('dispute_hold','refund_hold','payout_hold','risk_hold','manual_hold')),
  CONSTRAINT holds_v15_scope_type_ck CHECK (scope_type IN ('shop','user','platform','provider_account')),
  CONSTRAINT holds_v15_status_ck CHECK (status IN ('active','released','consumed','expired','review_required')),
  CONSTRAINT holds_v15_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT holds_v15_reason_nonempty CHECK (btrim(reason) <> ''),
  CONSTRAINT holds_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.holds_v15
  ADD CONSTRAINT uq_holds_v15_project_hold_key UNIQUE (project_id, hold_key);

CREATE INDEX IF NOT EXISTS idx_holds_v15_project_status_expires
  ON public.holds_v15(project_id, status, expires_at);

CREATE INDEX IF NOT EXISTS idx_holds_v15_project_scope_status
  ON public.holds_v15(project_id, scope_type, scope_id, status);

-- =========================
-- payouts (SoT + ledger link)
-- =========================
CREATE TABLE IF NOT EXISTS public.payouts_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  payout_key text NOT NULL,           -- stripe:po_... or internal:...
  provider text NOT NULL,
  provider_payout_id text NULL,

  shop_id text NOT NULL,
  currency text NOT NULL,

  amount_gross_minor bigint NOT NULL,
  amount_fee_minor bigint NULL,
  amount_net_minor bigint NOT NULL,

  status text NOT NULL,               -- scheduled|initiated|processing|completed|failed|retryable|review_required
  scheduled_for date NOT NULL,        -- date SoT (deadline)
  initiated_at timestamptz NULL,
  completed_at timestamptz NULL,
  failed_at timestamptz NULL,
  failure_code text NULL,
  failure_message text NULL,

  attempt_count int NOT NULL DEFAULT 0,
  next_retry_at timestamptz NULL,

  source_event_key varchar(128) NULL,
  posting_key char(64) NOT NULL,
  ledger_posting_id uuid NULL REFERENCES public.ledger_postings(id) ON DELETE SET NULL,

  related_hold_key text NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  meta jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT payouts_v15_status_ck CHECK (status IN ('scheduled','initiated','processing','completed','failed','retryable','review_required')),
  CONSTRAINT payouts_v15_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT payouts_v15_provider_nonempty CHECK (btrim(provider) <> ''),
  CONSTRAINT payouts_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.payouts_v15
  ADD CONSTRAINT uq_payouts_v15_project_payout_key UNIQUE (project_id, payout_key);

ALTER TABLE public.payouts_v15
  ADD CONSTRAINT uq_payouts_v15_project_posting_key UNIQUE (project_id, posting_key);

CREATE INDEX IF NOT EXISTS idx_payouts_v15_project_status_sched
  ON public.payouts_v15(project_id, status, scheduled_for);

CREATE INDEX IF NOT EXISTS idx_payouts_v15_project_shop_sched
  ON public.payouts_v15(project_id, shop_id, scheduled_for);

-- =========================
-- settlements (SoT)
-- =========================
CREATE TABLE IF NOT EXISTS public.settlement_batches_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  provider text NOT NULL,
  batch_key text NOT NULL,            -- stripe:settlement_report:YYYY-MM-DD
  status text NOT NULL,               -- open|reconciling|reconciled|review_required|failed_recorded

  from_at timestamptz NOT NULL,
  to_at timestamptz NOT NULL,

  artifact_ref text NULL,             -- pointer to report asset (external/FS)
  matched_count int NOT NULL DEFAULT 0,
  unmatched_count int NOT NULL DEFAULT 0,
  ambiguous_count int NOT NULL DEFAULT 0,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT sb_v15_status_ck CHECK (status IN ('open','reconciling','reconciled','review_required','failed_recorded')),
  CONSTRAINT sb_v15_provider_nonempty CHECK (btrim(provider) <> ''),
  CONSTRAINT sb_v15_batch_key_nonempty CHECK (btrim(batch_key) <> ''),
  CONSTRAINT sb_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.settlement_batches_v15
  ADD CONSTRAINT uq_sb_v15_project_batch_key UNIQUE (project_id, batch_key);

CREATE INDEX IF NOT EXISTS idx_sb_v15_project_status_time
  ON public.settlement_batches_v15(project_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS public.settlement_items_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  batch_id uuid NOT NULL REFERENCES public.settlement_batches_v15(id) ON DELETE CASCADE,

  provider_object_id text NOT NULL,    -- balance_txn etc
  event_key varchar(128) NULL,         -- UTL event_key candidate
  posting_key char(64) NULL,           -- ledger posting_key candidate

  currency text NOT NULL,
  amount_minor bigint NOT NULL,

  match_status text NOT NULL,          -- matched|unmatched|ambiguous
  matched_posting_id uuid NULL REFERENCES public.ledger_postings(id) ON DELETE SET NULL,
  matched_event_key varchar(128) NULL,

  match_confidence numeric NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT si_v15_match_status_ck CHECK (match_status IN ('matched','unmatched','ambiguous')),
  CONSTRAINT si_v15_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT si_v15_provider_object_nonempty CHECK (btrim(provider_object_id) <> ''),
  CONSTRAINT si_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.settlement_items_v15
  ADD CONSTRAINT uq_si_v15_batch_provider_object UNIQUE (batch_id, provider_object_id);

CREATE INDEX IF NOT EXISTS idx_si_v15_batch_status
  ON public.settlement_items_v15(batch_id, match_status);

-- =========================
-- funds operations (Ops queue)
-- =========================
CREATE TABLE IF NOT EXISTS public.funds_operations_v15 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  op_key char(64) NOT NULL,            -- sha256(project|op_type|object_type|object_key|reason)
  op_type text NOT NULL,               -- dispute_review|refund_review|payout_review|settlement_review|hold_review
  object_type text NOT NULL,           -- dispute|refund|payout|settlement_batch|hold
  object_key text NOT NULL,

  severity text NOT NULL,              -- low|medium|high|critical
  status text NOT NULL,                -- open|acknowledged|resolved|suppressed

  reason text NOT NULL,
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT fo_v15_op_key_len CHECK (length(op_key) = 64),
  CONSTRAINT fo_v15_op_type_ck CHECK (op_type IN ('dispute_review','refund_review','payout_review','settlement_review','hold_review')),
  CONSTRAINT fo_v15_object_type_ck CHECK (object_type IN ('dispute','refund','payout','settlement_batch','hold')),
  CONSTRAINT fo_v15_severity_ck CHECK (severity IN ('low','medium','high','critical')),
  CONSTRAINT fo_v15_status_ck CHECK (status IN ('open','acknowledged','resolved','suppressed')),
  CONSTRAINT fo_v15_reason_nonempty CHECK (btrim(reason) <> ''),
  CONSTRAINT fo_v15_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

ALTER TABLE public.funds_operations_v15
  ADD CONSTRAINT uq_fo_v15_project_op_key UNIQUE (project_id, op_key);

CREATE INDEX IF NOT EXISTS idx_fo_v15_project_status_time
  ON public.funds_operations_v15(project_id, status, created_at DESC);

COMMIT;