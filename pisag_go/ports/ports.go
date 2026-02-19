package ports

import (
	"context"
	"crypto/x509"
	"net/http"
	"time"
)

// Policy は SSRF 防御、正規化、および TLS 検証の制約を定義します。
type Policy struct {
	AllowedHosts []AllowedHost
	AllowCIDRs   []string // 開発/テスト用：prod環境では空を推奨
	MaxRedirects int
	Timeout      time.Duration

	// TLSRootCAs は信頼するルート証明書を明示的に指定します。
	// これにより、システム標準のルートCAをバイパスし、特定の内部認証局のみを許可できます。
	// nil の場合はシステムのルートCAが使用されます。
	TLSRootCAs *x509.CertPool
}

type AllowedHost struct {
	Host         string
	Port         int
	PathPrefixes []string
}

type RequestMeta struct {
	TraceID string
}

// FetchClient は ak_go_core/worker が依存する最小限のインターフェースです。
type FetchClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// TransportFactory は Policy に基づいて保護された Transport を生成します。
type TransportFactory interface {
	NewTransport(policy Policy) (*http.Transport, error)
}

type Fetcher interface {
	Fetch(ctx context.Context, url string, meta RequestMeta) ([]byte, error)
}