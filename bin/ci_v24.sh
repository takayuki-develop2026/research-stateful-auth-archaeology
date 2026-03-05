#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC="$ROOT/contracts/openapi/v1.yaml"
REL_DIR="$ROOT/contracts/openapi/releases"
ART_DIR="$ROOT/artifacts/contract"

mkdir -p "$ART_DIR" "$ROOT/artifacts/sdk" "$ROOT/artifacts/conformance"

echo "[v24] 1) Lint OpenAPI"
"$ROOT/bin/ci_v24_lint.sh"

echo "[v24] 2) Breaking-change diff vs latest release"
"$ROOT/bin/ci_v24_diff.sh"

echo "[v24] 3) SDK generation (Ruby/TS/Python) (optional but included)"
"$ROOT/bin/ci_v24_sdk.sh"

echo "[v24] 4) Conformance (DecisionCore running required)"
"$ROOT/bin/ci_v24_conformance.sh"

echo "[v24] OK"
