package worker

import (
	"context"
	"errors"
	"net/http"
	"time"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
	"example.com/pisag_go/usecase"
)

type PISAGHTTPFetcher struct {
	Policy ports.Policy
	Client ports.FetchClient // hardened client (pisag.NewClient)
	UserAgent string
}

func (f *PISAGHTTPFetcher) Fetch(ctx context.Context, targetURL string) (usecase.FetchResult, error) {
	// 使わない（互換のためだけ）
	return usecase.FetchResult{}, errors.New("FetchResult-only API not used in worker; use FetchBody")
}

// FetchBody returns *http.Response so worker can stream-save evidence.
func (f *PISAGHTTPFetcher) FetchBody(ctx context.Context, targetURL string) (*http.Response, error) {
	p := f.Policy
	if p.Timeout <= 0 {
		p.Timeout = 15 * time.Second
	}
	if p.MaxRedirects <= 0 {
		p.MaxRedirects = 3
	}

	u, err := pisag.RequestGuard(targetURL, p)
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
	ua := f.UserAgent
	if ua == "" {
		ua = "pisag-go/worker-v4.2"
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