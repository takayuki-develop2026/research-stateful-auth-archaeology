FROM golang:1.23-alpine AS build
WORKDIR /src

COPY services/opssvc/go.mod ./
RUN go mod download || true

COPY services/opssvc/ ./
RUN CGO_ENABLED=0 go build -o /out/opssvc ./cmd/opssvc

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/opssvc /app/opssvc
EXPOSE 9011
CMD ["/app/opssvc"]