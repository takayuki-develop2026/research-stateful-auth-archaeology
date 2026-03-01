FROM golang:1.23-alpine AS build
WORKDIR /src

COPY services/govsvc/go.mod ./
# go.sum が無いケースでも落ちない（依存が増えたら go mod tidy で生成される）
RUN go mod download || true

COPY services/govsvc/ ./
RUN CGO_ENABLED=0 go build -o /out/govsvc ./cmd/govsvc

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/govsvc /app/govsvc
EXPOSE 9012
CMD ["/app/govsvc"]