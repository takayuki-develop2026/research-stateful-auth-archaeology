#!/usr/bin/env bash
set -euo pipefail

SPEC="contracts/openapi/v1.yaml"
OUT="artifacts/sdk"
rm -rf "$OUT"
mkdir -p "$OUT"

gen() {
  local lang="$1"
  local outdir="$2"
  docker run --rm -v "$PWD:/work" -w /work openapitools/openapi-generator-cli:latest \
    generate -i "$SPEC" -g "$lang" -o "$outdir" \
    --skip-validate-spec
}

echo "[sdk] ruby"
gen ruby "$OUT/ruby"

echo "[sdk] typescript-fetch"
gen typescript-fetch "$OUT/typescript"

echo "[sdk] python"
gen python "$OUT/python"

echo "[sdk] OK => $OUT"
