FROM golang:1.22-alpine AS build
WORKDIR /src

# deps
COPY services/decisioncoresvc/go.mod services/decisioncoresvc/go.sum ./
RUN go mod download

# src
COPY services/decisioncoresvc/ ./

# build 3 binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/decisioncore_server ./cmd/decisioncore_server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/decisioncore_worker ./cmd/decisioncore_worker
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/v23_smoke ./cmd/v23_decisioncore_smoke

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/decisioncore_server /app/decisioncore_server
COPY --from=build /out/decisioncore_worker /app/decisioncore_worker
COPY --from=build /out/v23_smoke /app/v23_smoke

# default: server
ENTRYPOINT ["/app/decisioncore_server"]