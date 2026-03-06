FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wormexportersvc ./cmd/wormexportersvc

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata jq \
  && addgroup -S appgroup \
  && adduser -S appuser -G appgroup \
  && mkdir -p /var/wormexporter/out \
  && chown -R appuser:appgroup /var/wormexporter

WORKDIR /app
COPY --from=build /out/wormexportersvc /app/wormexportersvc
RUN chown appuser:appgroup /app/wormexportersvc && chmod 550 /app/wormexportersvc

USER appuser
ENTRYPOINT ["/app/wormexportersvc"]