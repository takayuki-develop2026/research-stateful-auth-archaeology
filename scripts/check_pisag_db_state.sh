#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "== runs =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "SELECT run_id, project_id, trace_id, status, started_at, finished_at, error_code, error_message FROM public.runs ORDER BY started_at DESC LIMIT 5;"

echo
echo "== run_inputs =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "SELECT id, run_id, claim_status, claimed_by, target_url, allowlist_key, attempt_count, claimed_at, next_attempt_at, created_at FROM public.run_inputs ORDER BY id DESC LIMIT 5;"

echo
echo "== run_evidence_assets =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "SELECT id, run_id, trace_id, kind, content_type, byte_size, sha256, final_url, stored_path, created_at FROM public.run_evidence_assets ORDER BY id DESC LIMIT 10;"

echo
echo "== run_evidence_manifests (schema) =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "\d+ public.run_evidence_manifests"

echo
echo "== run_evidence_manifests (rows) =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "SELECT * FROM public.run_evidence_manifests LIMIT 10;"

echo
echo "== run_events =="
docker compose exec -T ak_postgres psql -U ak -d ak -c "SELECT id, run_id, trace_id, event_name, step, status, message, created_at FROM public.run_events ORDER BY id DESC LIMIT 20;"
