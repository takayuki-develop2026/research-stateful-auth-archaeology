#!/usr/bin/env bash
set -euo pipefail

SPEC="contracts/openapi/v1.yaml"
REL_DIR="contracts/openapi/releases"
OUT_DIR="artifacts/contract"
mkdir -p "$OUT_DIR"

latest_release() {
  ls "$REL_DIR"/v*.yaml 2>/dev/null | \
    sed 's|.*/v||; s|\.yaml$||' | \
    sort -V | tail -n 1
}

BASE_VER="$(latest_release || true)"
if [[ -z "${BASE_VER:-}" ]]; then
  echo "[diff] No baseline in $REL_DIR. Skipping breaking check."
  exit 0
fi

BASE="$REL_DIR/v${BASE_VER}.yaml"
echo "[diff] base=$BASE (v${BASE_VER}) vs head=$SPEC"

# openapitools/openapi-diff supports --fail-on-incompatible
docker run --rm -v "$PWD:/work" -w /work openapitools/openapi-diff:latest \
  --fail-on-incompatible \
  "$BASE" "$SPEC" \
  | tee "$OUT_DIR/openapi_diff_v${BASE_VER}_to_head.txt"
