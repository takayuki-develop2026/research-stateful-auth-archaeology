package usecase

import (
	"context"
	"errors"
)

// Fetcher は外部へのフェッチを担当するインターフェース（StartFetchRunUseCase/worker が依存）
type Fetcher interface {
	Fetch(ctx context.Context, targetURL string) (FetchResult, error)
}

// KeyedFetcher は allowlist_key を必須にする（fail-closed 条文化）
type KeyedFetcher interface {
	FetchWithAllowlistKey(ctx context.Context, targetURL string, allowlistKey string) (FetchResult, error)
}

type FetchResult struct {
	FinalURL    string
	StatusCode  int
	ContentType string
	BodySize    int
}

// ErrDenied は allowlist/Policy による拒否を表す統一エラー
var ErrDenied = errors.New("fetch_denied")