FROM golang:1.23-alpine AS build
WORKDIR /src

COPY services/agentsvc/go.mod ./
RUN go mod download || true

COPY services/agentsvc/ ./
RUN CGO_ENABLED=0 go build -o /out/agentsvc ./cmd/agentsvc

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/agentsvc /app/agentsvc
EXPOSE 9010
CMD ["/app/agentsvc"]