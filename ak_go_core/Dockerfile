# Stage 1: Build
FROM golang:1.25.7-alpine3.23 AS build

WORKDIR /app

# --- 最強の防衛ポイント: 依存関係の物理コピー ---
# 親ディレクトリにある pisag_go と ak_go_core 両方の go.mod をコピー
COPY ak_go_core/go.mod ak_go_core/go.sum ./ak_go_core/
COPY pisag_go/go.mod pisag_go/go.sum ./pisag_go/

# ak_go_coreディレクトリに移動して依存関係をダウンロード
WORKDIR /app/ak_go_core
RUN go mod download

# 全ソースコードをコピー
WORKDIR /app
COPY . .

# 各バイナリのビルド (trimpath を使いビルド時のパス情報を消去して隠蔽)
WORKDIR /app/ak_go_core
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# Stage 2: Runtime (Hardened)
FROM alpine:3.23.3

# --- 防衛層: セキュリティパッチと特権排除 ---
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S appgroup \
  && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker

# --- 防衛層: ファイル書き込み権限の剥奪 ---
# バイナリを読み取り専用にし、所有権だけ変更
RUN chown appuser:appgroup /app/api /app/worker \
  && chmod 550 /app/api /app/worker

USER appuser

EXPOSE 9000
CMD ["/app/api"]