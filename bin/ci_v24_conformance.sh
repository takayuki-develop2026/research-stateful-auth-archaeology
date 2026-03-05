#!/usr/bin/env bash
set -euo pipefail

BASE="${DECISIONCORE_BASE_URL:-http://localhost:9023}"
PROJ_ID="${PROJ_ID:-akproj_0000000000000000000}"
TRC="trc_conformance_$(date +%s)"

echo "[conf] base=$BASE project=$PROJ_ID trace=$TRC"

# 1) health must be ok:true
H=$(curl -sS "$BASE/health")
echo "[conf] health=$H"
echo "$H" | grep -q '"ok":true'

# 2) error response shape
E=$(curl -sS -X POST "$BASE/v1/projects/$PROJ_ID/policy/evaluate" \
  -H "Content-Type: application/json" -H "X-Trace-Id: $TRC" --data '{}')
echo "[conf] error=$E"
echo "$E" | grep -q '"error"'
echo "$E" | grep -q '"type"'
echo "$E" | grep -q '"message"'
echo "$E" | grep -q '"trace_id"'

# 3) success evaluate shape (uses known evidence ids in your env)
OK=$(curl -sS -X POST "$BASE/v1/projects/$PROJ_ID/policy/evaluate" \
  -H "Content-Type: application/json" -H "X-Trace-Id: $TRC" --data '{
    "run_id":"00000000-0000-0000-0000-000000000000",
    "policy_version_str":"v23",
    "pipeline_version":"v23",
    "inputs_evidence_asset_id":68,
    "reason_evidence_asset_id":69,
    "obligations_evidence_asset_id":44
  }')
echo "[conf] ok=$OK"
echo "$OK" | grep -q '"policy_evaluation_id"'
echo "$OK" | grep -q '"input_hash"'
echo "$OK" | grep -q '"result"'
echo "$OK" | grep -q "\"trace_id\":\"$TRC\""

# Save report artifact (simple json bundle)
mkdir -p artifacts/conformance
cat > artifacts/conformance/report.json <<JSON
{
  "base": "$BASE",
  "project_id": "$PROJ_ID",
  "trace_id": "$TRC",
  "health": $H,
  "error_response": $E,
  "evaluate_ok": $OK,
  "status": "passed"
}
JSON

echo "[conf] PASSED"
