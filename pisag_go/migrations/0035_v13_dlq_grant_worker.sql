-- migrations/0035_v13_dlq_grant_worker.sql
-- Allow ak_worker to enqueue DLQ items (EXECUTE ONLY; still no table direct access)

BEGIN;

GRANT EXECUTE ON FUNCTION public.dlq_enqueue_v13(
  varchar, uuid, uuid, varchar, varchar, varchar, bigint, bigint
) TO ak_worker;

GRANT EXECUTE ON FUNCTION public.dlq_mark_v13(
  varchar, bigint, varchar, bigint
) TO ak_worker;

COMMIT;