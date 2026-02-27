package worker

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
	"example.com/pisag_go/usecase"
)

type PISAGHTTPFetcher struct {
	Policy    ports.Policy
	Client    ports.FetchClient
	UserAgent string
}

// FetchBodyWithAllowlistKey returns *http.Response so worker can stream-save evidence.
// v4 rule: allowlist_key is fail-closed (empty => deny).
func (f *PISAGHTTPFetcher) FetchBodyWithAllowlistKey(ctx context.Context, targetURL string, allowlistKey string) (*http.Response, error) {
	if strings.TrimSpace(allowlistKey) == "" {
		return nil, errors.Join(usecase.ErrDenied, pisag.ErrAllowlistKeyRequired)
	}

	p := f.Policy
	if p.Timeout <= 0 {
		p.Timeout = 15 * time.Second
	}
	if p.MaxRedirects <= 0 {
		p.MaxRedirects = 3
	}

	u, err := pisag.RequestGuardWithAllowlistKey(targetURL, p, allowlistKey)
	if err != nil {
		return nil, errors.Join(usecase.ErrDenied, err)
	}

	client := f.Client
	if client == nil {
		c, cerr := pisag.NewClient(p)
		if cerr != nil {
			return nil, cerr
		}
		client = c
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	ua := strings.TrimSpace(f.UserAgent)
	if ua == "" {
		ua = "pisag-go/worker-v4.5"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, pisag.ErrRedirectNotAllowed) || errors.Is(err, pisag.ErrIPNotAllowed) {
			return nil, errors.Join(usecase.ErrDenied, err)
		}
		return nil, err
	}
	return resp, nil
}
