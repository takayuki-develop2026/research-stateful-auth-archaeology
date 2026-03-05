FROM golang:1.22-alpine

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN which go && go version

ENTRYPOINT ["sh","-c","go test ./... -run TestV21 -v"]