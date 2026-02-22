package usecase

import (
	"context"
	"errors"
)

// Fetcher は外部へのフェッチを担当（StartFetchRunUseCase が依存）
// - 本番: PISAGFetcher
// - テスト: FakeFetcher
type Fetcher interface {
	Fetch(ctx context.Context, targetURL string) (FetchResult, error)
}

type FetchResult struct {
	FinalURL    string
	StatusCode  int
	ContentType string
	BodySize    int
}

// ErrDenied は「PISAGポリシーにより拒否」されたことを示す。
// StartFetchRunUseCase は errors.Is(err, ErrDenied) で判定する。
var ErrDenied = errors.New("fetch_denied")