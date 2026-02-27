package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
)

// PISAGFetcher is a hardened fetcher:
// RequestGuardWithAllowlistKey -> hardened Client -> GET -> MaxBodyBytes -> FetchResult.
//
// v4 fixed:
// - allowlist_key is REQUIRED (fail-closed).
type PISAGFetcher struct {
	Policy ports.Policy

	// Optional: injected client for tests; if nil, NewClient(policy) is used.
	Client ports.FetchClient

	// Hard cap to avoid OOM on large responses (0 => default 5MB).
	MaxBodyBytes int64

	// Optional: user-agent override (empty => default).
	UserAgent string
}

// Fetch (legacy) is intentionally fail-closed.
// If you need fetch, call FetchWithAllowlistKey.
func (f *PISAGFetcher) Fetch(ctx context.Context, targetURL string) (FetchResult, error) {
	return FetchResult{}, errors.Join(ErrDenied, fmt.Errorf("allowlist_key is required: use FetchWithAllowlistKey"))
}

func (f *PISAGFetcher) FetchWithAllowlistKey(ctx context.Context, targetURL string, allowlistKey string) (FetchResult, error) {
	_, res, err := f.FetchBytesWithAllowlistKey(ctx, targetURL, allowlistKey)
	return res, err
}

// FetchBytesWithAllowlistKey returns body bytes + metadata.
func (f *PISAGFetcher) FetchBytesWithAllowlistKey(ctx context.Context, targetURL string, allowlistKey string) ([]byte, FetchResult, error) {
	p := f.Policy
	applyPolicyDefaults(&p)

	// 1) URL normalize + allowlist enforce + allowlist_key fail-closed
	u, err := pisag.RequestGuardWithAllowlistKey(targetURL, p, allowlistKey)
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
	ua := strings.TrimSpace(f.UserAgent)
	if ua == "" {
		ua = "pisag-go/v4.5"
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
		ContentType: strings.TrimSpace(resp.Header.Get("Content-Type")),
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
