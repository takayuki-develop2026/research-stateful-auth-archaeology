-- 0027_v18_artifact_register_fns.sql
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- idempotency records for artifact register
-- ============================================================
CREATE TABLE IF NOT EXISTS public.artifact_idempotency_records (
  id               bigserial PRIMARY KEY,
  project_id        varchar(26) NOT NULL,
  scope             varchar(64) NOT NULL,  -- fixed: 'artifact_register_v18'
  idempotency_key   text        NOT NULL,
  artifact_ref      uuid        NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT artifact_idem_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT artifact_idem_scope_nonempty CHECK (btrim(scope::text) <> ''),
  CONSTRAINT artifact_idem_key_nonempty CHECK (btrim(idempotency_key::text) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ux_artifact_idem_project_scope_key'
  ) THEN
    ALTER TABLE public.artifact_idempotency_records
      ADD CONSTRAINT ux_artifact_idem_project_scope_key
      UNIQUE (project_id, scope, idempotency_key);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'artifact_idem_project_id_fkey'
  ) THEN
    ALTER TABLE public.artifact_idempotency_records
      ADD CONSTRAINT artifact_idem_project_id_fkey
      FOREIGN KEY (project_id)
      REFERENCES public.projects(project_id)
      ON DELETE CASCADE;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_artifact_idem_project_time
  ON public.artifact_idempotency_records(project_id, created_at DESC);

-- ============================================================
-- artifact_register_v18
--   returns: artifact_ref, found_existing
-- ============================================================
CREATE OR REPLACE FUNCTION public.artifact_register_v18(
  p_project_id      varchar,
  p_artifact_type   varchar,
  p_schema_version  varchar,
  p_content_sha256  text,
  p_content_length  bigint,
  p_mime_type       varchar,
  p_status          varchar,  -- NOTE: default禁止（後ろに必須引数があるため）
  p_idempotency_key text
)
RETURNS TABLE (
  artifact_ref    uuid,
  found_existing  boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_existing uuid;
  v_scope text := 'artifact_register_v18';
  v_idem_ref uuid;
  v_status varchar;
  v_sha text;
BEGIN
  -- basic validation
  IF p_project_id IS NULL OR btrim(p_project_id::text) = '' THEN
    RAISE EXCEPTION 'project_id is required';
  END IF;

  IF p_artifact_type IS NULL OR btrim(p_artifact_type::text) = '' THEN
    RAISE EXCEPTION 'artifact_type is required';
  END IF;

  IF p_schema_version IS NULL OR btrim(p_schema_version::text) = '' THEN
    RAISE EXCEPTION 'schema_version is required';
  END IF;

  IF p_content_length IS NULL OR p_content_length <= 0 THEN
    RAISE EXCEPTION 'content_length must be > 0';
  END IF;

  IF p_mime_type IS NULL OR btrim(p_mime_type::text) = '' THEN
    RAISE EXCEPTION 'mime_type is required';
  END IF;

  IF p_idempotency_key IS NULL OR btrim(p_idempotency_key::text) = '' THEN
    RAISE EXCEPTION 'idempotency_key is required';
  END IF;

  -- normalize status (empty -> 'active')
  v_status := COALESCE(NULLIF(btrim(COALESCE(p_status,'')), ''), 'active');

  -- normalize sha (empty -> NULL)
  v_sha := NULLIF(btrim(COALESCE(p_content_sha256, '')::text), '');

  -- 0) idempotency check first
  SELECT air.artifact_ref
    INTO v_idem_ref
  FROM public.artifact_idempotency_records air
  WHERE air.project_id = p_project_id
    AND air.scope = v_scope
    AND air.idempotency_key = p_idempotency_key
  LIMIT 1;

  IF v_idem_ref IS NOT NULL THEN
    RETURN QUERY SELECT v_idem_ref, TRUE;
    RETURN;
  END IF;

  -- 1) sha-based de-dup (project-wide) if sha present
  IF v_sha IS NOT NULL THEN
    SELECT aa.artifact_ref
      INTO v_existing
    FROM public.artifact_assets aa
    WHERE aa.project_id = p_project_id
      AND aa.content_sha256 = v_sha
    LIMIT 1;

    IF v_existing IS NOT NULL THEN
      INSERT INTO public.artifact_idempotency_records
        (project_id, scope, idempotency_key, artifact_ref)
      VALUES
        (p_project_id, v_scope, p_idempotency_key, v_existing)
      ON CONFLICT (project_id, scope, idempotency_key) DO NOTHING;

      RETURN QUERY SELECT v_existing, TRUE;
      RETURN;
    END IF;
  END IF;

  -- 2) create new artifact_assets row
  v_existing := gen_random_uuid();

  INSERT INTO public.artifact_assets
    (project_id, artifact_ref, artifact_type, schema_version,
     content_sha256, content_length, mime_type, status,
     created_at, updated_at)
  VALUES
    (p_project_id, v_existing, p_artifact_type, p_schema_version,
     v_sha, p_content_length, p_mime_type, v_status,
     now(), now());

  -- 3) record idempotency
  INSERT INTO public.artifact_idempotency_records
    (project_id, scope, idempotency_key, artifact_ref)
  VALUES
    (p_project_id, v_scope, p_idempotency_key, v_existing)
  ON CONFLICT (project_id, scope, idempotency_key) DO NOTHING;

  RETURN QUERY SELECT v_existing, FALSE;
END;
$$;

REVOKE ALL ON FUNCTION public.artifact_register_v18(
  varchar, varchar, varchar, text, bigint, varchar, varchar, text
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.artifact_register_v18(
  varchar, varchar, varchar, text, bigint, varchar, varchar, text
) TO ak_worker;

COMMIT;