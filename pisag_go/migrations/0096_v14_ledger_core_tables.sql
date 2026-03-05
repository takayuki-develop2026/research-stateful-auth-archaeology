-- 0096_v14_ledger_core_tables.sql
-- v14.0 Ledger SoT core: ledger_accounts, ledger_postings, ledger_entries
-- Policy:
-- - double-entry enforced at finalize step (DB-side)
-- - idempotent by (project_id, posting_key)
-- - 1 account = 1 currency
-- - do NOT auto-create unknown accounts (enforced in exec-only insert)

BEGIN;

-- pgcrypto for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================
-- ENUM TYPES (idempotent)
-- =========================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_account_type_v14') THEN
    CREATE TYPE ledger_account_type_v14 AS ENUM ('asset','liability','equity','revenue','expense');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_owner_type_v14') THEN
    CREATE TYPE ledger_owner_type_v14 AS ENUM ('shop','platform','provider','user','system');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_account_status_v14') THEN
    CREATE TYPE ledger_account_status_v14 AS ENUM ('active','archived');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_posting_type_v14') THEN
    CREATE TYPE ledger_posting_type_v14 AS ENUM ('sale','refund','fee','payout','adjustment','reserve','release');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_posting_status_v14') THEN
    CREATE TYPE ledger_posting_status_v14 AS ENUM ('draft','posted','review_required','failed_recorded','voided');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_direction_v14') THEN
    CREATE TYPE ledger_direction_v14 AS ENUM ('debit','credit');
  END IF;
END$$;

-- =========================
-- TABLE: ledger_accounts
-- =========================
CREATE TABLE IF NOT EXISTS ledger_accounts (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id    text NOT NULL,

  account_key   text NOT NULL, -- e.g. "provider:stripe:clearing", "platform:revenue:sales", "shop:123:cash"
  account_type  ledger_account_type_v14 NOT NULL,

  currency      text NOT NULL, -- 1 account = 1 currency
  owner_type    ledger_owner_type_v14 NOT NULL,
  owner_id      text NULL,

  status        ledger_account_status_v14 NOT NULL DEFAULT 'active',

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ledger_accounts_account_key_len CHECK (length(account_key) > 0),
  CONSTRAINT ledger_accounts_currency_len CHECK (length(currency) > 0)
);

-- unique: no duplicate account key in a project
CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_accounts_project_account_key
  ON ledger_accounts(project_id, account_key);

CREATE INDEX IF NOT EXISTS ix_ledger_accounts_project_owner
  ON ledger_accounts(project_id, owner_type, owner_id);

CREATE INDEX IF NOT EXISTS ix_ledger_accounts_project_status
  ON ledger_accounts(project_id, status);

-- =========================
-- TABLE: ledger_postings
-- =========================
CREATE TABLE IF NOT EXISTS ledger_postings (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id       text NOT NULL,

  posting_key      text NOT NULL,        -- v6規約の冪等キー
  source_event_key text NOT NULL,        -- UTL event_key
  posting_type     ledger_posting_type_v14 NOT NULL,
  currency         text NOT NULL,

  status           ledger_posting_status_v14 NOT NULL DEFAULT 'draft',

  posted_at        timestamptz NOT NULL, -- 会計日付（close軸）
  run_id           text NOT NULL,
  trace_id         text NOT NULL,
  policy_version_id text NOT NULL,       -- published only (checked at app/policy layer)

  evidence_refs    jsonb NOT NULL DEFAULT '[]'::jsonb, -- array of evidence_asset_id

  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ledger_postings_posting_key_len CHECK (length(posting_key) > 0),
  CONSTRAINT ledger_postings_source_event_key_len CHECK (length(source_event_key) > 0),
  CONSTRAINT ledger_postings_currency_len CHECK (length(currency) > 0),
  CONSTRAINT ledger_postings_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs) = 'array')
);

-- unique: idempotency
CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_postings_project_posting_key
  ON ledger_postings(project_id, posting_key);

CREATE INDEX IF NOT EXISTS ix_ledger_postings_project_posted_at
  ON ledger_postings(project_id, posted_at);

CREATE INDEX IF NOT EXISTS ix_ledger_postings_project_source_event_key
  ON ledger_postings(project_id, source_event_key);

CREATE INDEX IF NOT EXISTS ix_ledger_postings_project_type_posted_at
  ON ledger_postings(project_id, posting_type, posted_at);

-- =========================
-- TABLE: ledger_entries
-- =========================
CREATE TABLE IF NOT EXISTS ledger_entries (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id   text NOT NULL,

  posting_id   uuid NOT NULL REFERENCES ledger_postings(id) ON DELETE RESTRICT,
  account_id   uuid NOT NULL REFERENCES ledger_accounts(id) ON DELETE RESTRICT,

  direction    ledger_direction_v14 NOT NULL,
  amount       bigint NOT NULL,      -- minor units
  currency     text NOT NULL,

  entry_key    text NOT NULL,        -- unique within posting
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  created_at   timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ledger_entries_amount_nonneg CHECK (amount >= 0),
  CONSTRAINT ledger_entries_currency_len CHECK (length(currency) > 0),
  CONSTRAINT ledger_entries_entry_key_len CHECK (length(entry_key) > 0),
  CONSTRAINT ledger_entries_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs) = 'array')
);

-- unique within posting
CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_entries_posting_entry_key
  ON ledger_entries(posting_id, entry_key);

CREATE INDEX IF NOT EXISTS ix_ledger_entries_project_account_created
  ON ledger_entries(project_id, account_id, created_at);

CREATE INDEX IF NOT EXISTS ix_ledger_entries_project_posting
  ON ledger_entries(project_id, posting_id);

COMMIT;