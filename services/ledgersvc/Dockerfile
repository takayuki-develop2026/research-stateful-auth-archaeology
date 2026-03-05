FROM golang:1.22-alpine AS build
WORKDIR /app
RUN apk add --no-cache ca-certificates git

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/v14_ledger_smoke ./cmd/v14_ledger_smoke
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/v141_ingest_smoke ./cmd/v141_ingest_smoke
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/v1411_utl_to_ledger_smoke ./cmd/v1411_utl_to_ledger_smoke

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/v14_ledger_smoke ./v14_ledger_smoke
COPY --from=build /out/v141_ingest_smoke ./v141_ingest_smoke
COPY --from=build /out/v1411_utl_to_ledger_smoke ./v1411_utl_to_ledger_smoke

ENV PROJECT_ID=akproj_0000000000000000000 \
    POLICY_VERSION_ID=policy_v1_published \
    CURRENCY=JPY

ENTRYPOINT ["./v14_ledger_smoke"]