package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
)

// PISAGFetcher is a hardened fetcher:
// RequestGuard (normalize+allowlist) -> hardened Client(Transport) -> GET -> MaxBodyBytes -> FetchResult.
//
// Important:
// - This fetcher NEVER reads env vars.
// - TLS behavior is controlled only by Policy.TLSRootCAs (nil => system roots).
// - All host/port/path/ip/redirect constraints are enforced by pisag.RequestGuard + pisag.NewClient policy.
// - Any policy/allowlist denial is normalized to ErrDenied (wrapped with original cause).
type PISAGFetcher struct {
	Policy ports.Policy

	// Optional: injected client for tests; if nil, NewClient(policy) is used.
	Client ports.FetchClient

	// Hard cap to avoid OOM on large responses (0 => default 5MB).
	MaxBodyBytes int64

	// Optional: user-agent override (empty => default).
	UserAgent string
}

// Fetch returns only metadata (drains body). Kept for backward compatibility.
func (f *PISAGFetcher) Fetch(ctx context.Context, targetURL string) (FetchResult, error) {
	_, res, err := f.FetchBytes(ctx, targetURL)
	return res, err
}

// FetchBytes returns body bytes + metadata.
// v4.3 evidence saver uses this.
func (f *PISAGFetcher) FetchBytes(ctx context.Context, targetURL string) ([]byte, FetchResult, error) {
	p := f.Policy
	applyPolicyDefaults(&p)

	// 1) URL normalize + allowlist enforce (single source of truth)
	u, err := pisag.RequestGuard(targetURL, p)
	if err != nil {
		return nil, FetchResult{}, errors.Join(ErrDenied, err)
	}

	// 2) Prepare client (hardened transport)
	var client ports.FetchClient = f.Client
	if client == nil {
		c, cerr := pisag.NewClient(p)
		if cerr != nil {
			return nil, FetchResult{}, cerr
		}
		client = c
	}

	// 3) Build request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, FetchResult{}, err
	}
	ua := f.UserAgent
	if ua == "" {
		ua = "pisag-go/v4.1"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")

	// 4) Do request
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, pisag.ErrRedirectNotAllowed) || errors.Is(err, pisag.ErrIPNotAllowed) {
			return nil, FetchResult{}, errors.Join(ErrDenied, err)
		}
		return nil, FetchResult{}, err
	}
	defer resp.Body.Close()

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	// 5) Read body up to limit
	limit := f.MaxBodyBytes
	if limit <= 0 {
		limit = 5 << 20 // 5MB
	}
	b, n, rerr := readAllWithLimit(resp.Body, limit)
	if rerr != nil {
		return nil, FetchResult{}, rerr
	}

	res := FetchResult{
		FinalURL:    finalURL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		BodySize:    int(n),
	}
	return b, res, nil
}

func applyPolicyDefaults(p *ports.Policy) {
	if p.MaxRedirects <= 0 {
		p.MaxRedirects = 3
	}
	if p.Timeout <= 0 {
		p.Timeout = 15 * time.Second
	}
}

// readAllWithLimit reads up to max bytes (+1 to detect overflow).
func readAllWithLimit(r io.Reader, max int64) ([]byte, int64, error) {
	if max <= 0 {
		return nil, 0, fmt.Errorf("invalid max body bytes: %d", max)
	}
	lr := &io.LimitedReader{R: r, N: max + 1}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, 0, err
	}
	n := int64(len(b))
	if n > max {
		return nil, n, fmt.Errorf("response too large: %d bytes (limit %d)", n, max)
	}
	return b, n, nil
}