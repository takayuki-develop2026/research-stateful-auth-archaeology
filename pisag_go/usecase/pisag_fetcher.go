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
// - Any policy/allowlist denial is normalized to ErrDenied (optionally wrapped with the original error).
type PISAGFetcher struct {
	Policy ports.Policy

	// Optional: injected client for tests; if nil, NewClient(policy) is used.
	Client ports.FetchClient

	// Hard cap to avoid OOM on large responses (0 => default 5MB).
	MaxBodyBytes int64

	// Optional: user-agent override (empty => default).
	UserAgent string
}

func (f *PISAGFetcher) Fetch(ctx context.Context, targetURL string) (FetchResult, error) {
	p := f.Policy
	applyPolicyDefaults(&p)

	// 1) URL normalize + allowlist enforce (single source of truth)
	u, err := pisag.RequestGuard(targetURL, p)
	if err != nil {
		// Deny should be surfaced as ErrDenied while keeping the original cause for debugging.
		return FetchResult{}, errors.Join(ErrDenied, err)
	}

	// 2) Prepare client (hardened transport)
	var client ports.FetchClient = f.Client
	if client == nil {
		c, cerr := pisag.NewClient(p)
		if cerr != nil {
			// Infra/setup error (not a deny)
			return FetchResult{}, cerr
		}
		client = c
	}

	// 3) Build request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FetchResult{}, err
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
		// Policy-related rejects should become ErrDenied.
		// (Redirect host change / IP not allowed etc.)
		if errors.Is(err, pisag.ErrRedirectNotAllowed) || errors.Is(err, pisag.ErrIPNotAllowed) {
			return FetchResult{}, errors.Join(ErrDenied, err)
		}
		return FetchResult{}, err
	}
	defer resp.Body.Close()

	// 5) Drain body up to limit (we only report size in FetchResult; content storage is v4.3+ evidence layer)
	limit := f.MaxBodyBytes
	if limit <= 0 {
		limit = 5 << 20 // 5MB
	}
	n, derr := drainWithLimit(resp.Body, limit)
	if derr != nil {
		return FetchResult{}, derr
	}

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	return FetchResult{
		FinalURL:    finalURL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		BodySize:    int(n),
	}, nil
}

func applyPolicyDefaults(p *ports.Policy) {
	if p.MaxRedirects <= 0 {
		p.MaxRedirects = 3
	}
	if p.Timeout <= 0 {
		p.Timeout = 15 * time.Second
	}
	// TLSRootCAs: nil means "use system roots" (handled inside pisag.NewTransport/NewClient).
}

func drainWithLimit(r io.Reader, max int64) (int64, error) {
	if max <= 0 {
		return 0, fmt.Errorf("invalid max body bytes: %d", max)
	}
	// +1 to detect overflow
	lr := &io.LimitedReader{R: r, N: max + 1}
	n, err := io.Copy(io.Discard, lr)
	if err != nil {
		return 0, err
	}
	if n > max {
		return n, fmt.Errorf("response too large: %d bytes (limit %d)", n, max)
	}
	return n, nil
}