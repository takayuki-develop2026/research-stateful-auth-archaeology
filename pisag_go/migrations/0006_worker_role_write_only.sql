DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_worker') THEN
    CREATE ROLE ak_worker LOGIN PASSWORD 'xxxx';
  ELSE
    ALTER ROLE ak_worker LOGIN PASSWORD 'xxxx';
  END IF;
END $$;

-- DB接続許可（必要なら）
GRANT CONNECT ON DATABASE ak TO ak_worker;

-- schema利用許可
GRANT USAGE ON SCHEMA public TO ak_worker;

-- まず “全部禁止” を明示（保険）
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ak_worker;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM ak_worker;

-- =========================
-- runs: INSERT + UPDATE only
-- =========================
GRANT INSERT, UPDATE ON public.runs TO ak_worker;

-- run_inputs: INSERT only（ON CONFLICT DO NOTHING を使う想定）
GRANT INSERT ON public.run_inputs TO ak_worker;

-- run_events: INSERT only
GRANT INSERT ON public.run_events TO ak_worker;

-- sequences: SERIAL/BIGSERIAL を使ってるテーブルのみ必要
-- run_inputs.id が bigserial などならこれが必須
GRANT USAGE, SELECT ON SEQUENCE public.run_inputs_id_seq TO ak_worker;

-- ✅ “SELECT拒否” を確実に（明示）
REVOKE SELECT ON public.runs FROM ak_worker;
REVOKE SELECT ON public.run_inputs FROM ak_worker;
REVOKE SELECT ON public.run_events FROM ak_worker;

-- 将来テーブル追加時にも write-only を維持（owner=ak を想定）
-- これをやるなら、実際の owner ロールで実行してください。
-- ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT ON TABLES FROM ak_worker;
-- ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT INSERT, UPDATE ON TABLES TO ak_worker;