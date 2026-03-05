#!/usr/bin/env bash
set -euo pipefail
SPEC="contracts/openapi/v1.yaml"

docker run --rm -v "$PWD:/work" -w /work redocly/cli:latest \
  lint "$SPEC"
