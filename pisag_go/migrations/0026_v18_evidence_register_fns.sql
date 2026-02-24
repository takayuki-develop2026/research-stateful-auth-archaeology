-- migrations/0026_v18_evidence_register_fns.sql
-- v18: Evidence registry - Register (idempotent) functions
-- - JSONゼロ方針: DBはメタ/ハッシュ/状態/参照のみ。本文は別ストア(S3等) + content_sha256で照合。
-- - Registerは「メタ台帳化」。実体アップロードは別経路（後続で issue-url / gateway 追加）。
--
-- Depends:
-- - pgcrypto (gen_random_uuid)
-- - projects(id varchar(26))
-- - evidence_assets(project_id, evidence_ref) unique (created in 0019_v18_evidence_assets.sql 想定)
--
-- Notes:
-- - content_sha256 は 64hex 想定（検査は軽め）
-- - Idempotency-Keyは v13 で統一するが、v18単体でも動くよう関数側で key を受け取る
-- - 重複排除方針：推奨は project_id + content_sha256 を “同一証拠” とみなして再利用（ON）
--   -> ただし「同一shaでも別refが欲しい」運用に変える可能性があるなら、下の UNIQUE を外して方針変更

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================================================
-- Helper: validate sha256 hex (lightweight)
-- =========================================================
CREATE OR REPLACE FUNCTION public.is_hex_sha256(p_sha text)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
  IF p_sha IS NULL THEN
    RETURN false;
  END IF;
  IF length(p_sha) <> 64 THEN
    RETURN false;
  END IF;
  -- simple regex: only 0-9a-f
  IF p_sha !~ '^[0-9a-f]{64}$' THEN
    RETURN false;
  END IF;
  RETURN true;
END;
$$;

REVOKE ALL ON FUNCTION public.is_hex_sha256(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.is_hex_sha256(text) TO PUBLIC;

-- =========================================================
-- Evidence register function (idempotent by content_sha256)
-- Returns existing evidence_ref if same (project_id, content_sha256) already registered.
-- =========================================================
CREATE OR REPLACE FUNCTION public.evidence_register_v18(
  p_project_id      varchar,
  p_trace_id        varchar,
  p_actor_type      varchar,
  p_actor_id        varchar,

  p_media_type      varchar,
  p_mime_type       varchar,
  p_source_kind     varchar,
  p_source_uri      text,

  p_content_sha256  text,
  p_content_length  bigint,
  p_language        varchar,

  p_retention_policy varchar,
  p_expires_at_utc   timestamptz,

  -- optional idempotency key (string)
  p_idempotency_key text
)
RETURNS TABLE (
  evidence_ref uuid,
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_existing uuid;
  v_new uuid;
BEGIN
  -- -------------------------
  -- required checks (hard)
  -- -------------------------
  IF btrim(coalesce(p_project_id,'')) = '' THEN
    RAISE EXCEPTION 'project_id is required';
  END IF;
  IF btrim(coalesce(p_trace_id,'')) = '' THEN
    RAISE EXCEPTION 'trace_id is required';
  END IF;
  IF btrim(coalesce(p_actor_type,'')) = '' THEN
    RAISE EXCEPTION 'actor_type is required';
  END IF;

  IF btrim(coalesce(p_media_type,'')) = '' THEN
    RAISE EXCEPTION 'media_type is required';
  END IF;
  IF btrim(coalesce(p_mime_type,'')) = '' THEN
    RAISE EXCEPTION 'mime_type is required';
  END IF;
  IF btrim(coalesce(p_source_kind,'')) = '' THEN
    RAISE EXCEPTION 'source_kind is required';
  END IF;

  IF NOT public.is_hex_sha256(lower(btrim(coalesce(p_content_sha256,'')))) THEN
    RAISE EXCEPTION 'content_sha256 must be 64 lowercase hex';
  END IF;
  IF p_content_length IS NULL OR p_content_length <= 0 THEN
    RAISE EXCEPTION 'content_length must be > 0';
  END IF;

  -- -------------------------
  -- validate domains (light)
  -- -------------------------
  IF p_actor_type NOT IN ('system','user','service') THEN
    RAISE EXCEPTION 'actor_type domain violation';
  END IF;

  IF p_retention_policy IS NULL OR btrim(p_retention_policy) = '' THEN
    p_retention_policy := 'standard';
  END IF;
  IF p_retention_policy NOT IN ('short','standard','legal_hold') THEN
    RAISE EXCEPTION 'retention_policy domain violation';
  END IF;

  -- media type domain
  IF p_media_type NOT IN ('text','image','audio','video','binary') THEN
    RAISE EXCEPTION 'media_type domain violation';
  END IF;

  -- -------------------------
  -- ensure project exists
  -- -------------------------
  PERFORM 1 FROM public.projects WHERE id = p_project_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'project not found: %', p_project_id;
  END IF;

  -- -------------------------
  -- de-dup by (project_id, content_sha256)
  -- -------------------------
  SELECT ea.evidence_ref
    INTO v_existing
  FROM public.evidence_assets ea
  WHERE ea.project_id = p_project_id
    AND ea.content_sha256 = lower(btrim(p_content_sha256))
  LIMIT 1;

  IF v_existing IS NOT NULL THEN
    evidence_ref := v_existing;
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  -- new
  v_new := gen_random_uuid();

  INSERT INTO public.evidence_assets (
    project_id,
    evidence_ref,

    media_type,
    source_kind,
    source_uri,

    content_sha256,
    content_length,
    mime_type,
    language,

    retention_policy,
    expires_at_utc,
    status,

    created_by_type,
    created_by_id,

    created_at,
    updated_at
  ) VALUES (
    p_project_id,
    v_new,

    p_media_type,
    p_source_kind,
    NULLIF(btrim(coalesce(p_source_uri,'')) , ''),

    lower(btrim(p_content_sha256)),
    p_content_length,
    p_mime_type,
    NULLIF(btrim(coalesce(p_language,'')), ''),

    p_retention_policy,
    p_expires_at_utc,
    'active',

    p_actor_type,
    NULLIF(btrim(coalesce(p_actor_id,'')), ''),

    now(),
    now()
  );

  -- (optional) audit_events hook: v18では「DBで自動生成」はまだやらない（既存audit_events設計が混在するため）
  -- ただし最低限の監査は “呼び出し側”で append するのが安全（Laravel/Rails/Goで統一）

  evidence_ref := v_new;
  found_existing := false;
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.evidence_register_v18(
  varchar, varchar, varchar, varchar,
  varchar, varchar, varchar, text,
  text, bigint, varchar,
  varchar, timestamptz,
  text
) FROM PUBLIC;

-- 実行権限: 現段階は owner(ak) と、将来の service role に付与する想定
GRANT EXECUTE ON FUNCTION public.evidence_register_v18(
  varchar, varchar, varchar, varchar,
  varchar, varchar, varchar, text,
  text, bigint, varchar,
  varchar, timestamptz,
  text
) TO ak_worker;

COMMIT;