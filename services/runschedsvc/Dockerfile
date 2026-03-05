FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/runschedsvc ./cmd/runschedsvc

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/runschedsvc /app/runschedsvc
ENV TZ=UTC
ENTRYPOINT ["/app/runschedsvc"]