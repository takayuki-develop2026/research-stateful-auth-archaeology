#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx

docker run --rm -it \
  --network simulation1_network \
  -v "$ROOT_DIR":/src \
  -v go-build-cache:/root/.cache/go-build \
  -v go-mod-cache:/go/pkg/mod \
  -w /src/pisag_go \
  -e AK_DB_DSN="postgres://ak:ak@ak_postgres:5432/ak?sslmode=disable" \
  golang:1.25.7-alpine3.23 \
  sh -lc '
    apk add --no-cache ca-certificates git &&
    cp /src/docker/nginx/ssl/oracle.crt /usr/local/share/ca-certificates/oracle-local.crt &&
    update-ca-certificates &&
    /usr/local/go/bin/go run ./cmd/ak_go_worker/main.go
  '
