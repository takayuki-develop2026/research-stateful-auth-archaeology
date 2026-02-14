# Stage 1: Build (stable)
# NOTE: As of Feb 2026, golang official image lists Go 1.26 as RC tags; latest stable is 1.25.7. :contentReference[oaicite:4]{index=4}
FROM golang:1.25.7-alpine3.23 AS build

WORKDIR /app

# deps
COPY go.mod go.sum ./
RUN go mod download

# build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker

# Stage 2: Runtime (security patched)
# Alpine 3.23.3 released 2026-01-27. :contentReference[oaicite:5]{index=5}
FROM alpine:3.23.3

# Minimal TLS roots (future outbound HTTPS), and run as non-root
RUN apk add --no-cache ca-certificates \
  && addgroup -S appgroup \
  && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker

# Own + drop privileges
RUN chown appuser:appgroup /app/api
USER appuser

EXPOSE 9000
CMD ["/app/api"]