package opa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Decide(ctx context.Context, input any, policyPath string, actionClass ActionClass) (Decision, string /*cacheKey*/, error)
}

type HTTPClient struct {
	cfg   ClientConfig
	http  *http.Client
	cache *DecisionCache
}

func NewHTTPClient(cfg ClientConfig) *HTTPClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 800 * time.Millisecond
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 2 * time.Second
	}
	if cfg.CacheMaxItems <= 0 {
		cfg.CacheMaxItems = 5000
	}
	return &HTTPClient{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
		cache: NewDecisionCache(cfg.CacheTTL, cfg.CacheMaxItems),
	}
}

func stableJSONHash(v any) (hex64 string, raw []byte, err error) {
	raw, err = json.Marshal(v)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

func (c *HTTPClient) Decide(ctx context.Context, input any, policyPath string, actionClass ActionClass) (Decision, string, error) {
	policyPath = strings.TrimSpace(policyPath)
	if policyPath == "" {
		return Decision{Result: ResultError}, "", fmt.Errorf("policyPath required")
	}

	hash, raw, err := stableJSONHash(input)
	if err != nil {
		return Decision{Result: ResultError}, "", err
	}

	cacheKey := hash + "|" + policyPath
	if v, ok := c.cache.Get(cacheKey); ok {
		return v, cacheKey, nil
	}

	// PDP request payload (OPA style)
	reqBody := map[string]any{"input": json.RawMessage(raw)}
	bodyBytes, _ := json.Marshal(reqBody)

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/data/" + strings.TrimLeft(policyPath, "/")

	var lastErr error
	for attempt := 0; attempt <= c.cfg.RetryCount; attempt++ {
		dec, e := c.callOnce(ctx, url, bodyBytes)
		if e == nil {
			c.cache.Put(cacheKey, dec)
			return dec, cacheKey, nil
		}
		lastErr = e

		if !isRetryable(e) {
			break
		}
		select {
		case <-ctx.Done():
			return Decision{Result: ResultError}, cacheKey, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 80 * time.Millisecond):
		}
	}

	// fail-closed for high_risk, fail-open-ish for low_risk_read (but with mask obligations)
	if actionClass == HighRisk {
		return Decision{
			Result:      ResultDeny,
			ReasonCodes: []string{"pdp_unavailable_fail_closed"},
			Obligations: Obligations{RequireApproval: false, RequireEvidence: false, MaskRuleKey: ""},
		}, cacheKey, lastErr
	}

	if actionClass == LowRiskRead {
		return Decision{
			Result:      ResultAllow,
			ReasonCodes: []string{"pdp_unavailable_fail_open_mask"},
			Obligations: Obligations{MaskRuleKey: "mask_on_pdp_unavailable"},
		}, cacheKey, lastErr
	}

	return Decision{
		Result:      ResultDeny,
		ReasonCodes: []string{"pdp_unavailable_low_risk_write_deny"},
		Obligations: Obligations{},
	}, cacheKey, lastErr
}

func (c *HTTPClient) callOnce(ctx context.Context, url string, body []byte) (Decision, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return Decision{Result: ResultError}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Decision{Result: ResultError}, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 500 {
		return Decision{Result: ResultError}, fmt.Errorf("pdp 5xx: %d body=%s", resp.StatusCode, string(b))
	}
	if resp.StatusCode >= 400 {
		return Decision{Result: ResultError}, fmt.Errorf("pdp 4xx: %d body=%s", resp.StatusCode, string(b))
	}

	// =========================================================
	// OPA response shapes
	// =========================================================
	// (1) bool result:
	//   {"result": true}
	//   {"result": false}
	// ---------------------------------------------------------
	type opaBool struct {
		Result *bool `json:"result"`
	}
	var rb opaBool
	if err := json.Unmarshal(b, &rb); err == nil && rb.Result != nil {
		if *rb.Result {
			return Decision{
				Result:      ResultAllow,
				ReasonCodes: []string{"opa_bool_allow"},
				Obligations: Obligations{},
			}, nil
		}
		return Decision{
			Result:      ResultDeny,
			ReasonCodes: []string{"opa_bool_deny"},
			Obligations: Obligations{},
		}, nil
	}

	// (2) object result:
	//   { "result": { "allow": true, "reason_codes": [...], "obligations": {...}, "score": 1.0 } }
	//   { "result": { "result": "allow|deny|error|review_required|proposal_only", ... } }
	// ---------------------------------------------------------
	type opaObj struct {
		Result struct {
			Allow       *bool       `json:"allow"`
			Result      string      `json:"result"`
			ReasonCodes []string    `json:"reason_codes"`
			Obligations Obligations `json:"obligations"`
			Score       float64     `json:"score"`
		} `json:"result"`
	}

	var a opaObj
	if err := json.Unmarshal(b, &a); err == nil {
		// default empty to avoid nil surprises
		rc := a.Result.ReasonCodes
		if rc == nil {
			rc = []string{}
		}
		ob := a.Result.Obligations // zero value ok

		// allow bool takes precedence
		if a.Result.Allow != nil {
			if *a.Result.Allow {
				return Decision{Result: ResultAllow, ReasonCodes: rc, Obligations: ob, Score: a.Result.Score}, nil
			}
			return Decision{Result: ResultDeny, ReasonCodes: rc, Obligations: ob, Score: a.Result.Score}, nil
		}

		// string result
		if strings.TrimSpace(a.Result.Result) != "" {
			dr := DecisionResult(strings.TrimSpace(a.Result.Result))
			switch dr {
			case ResultAllow, ResultDeny, ResultError, ResultReviewRequired, ResultProposalOnly:
				return Decision{Result: dr, ReasonCodes: rc, Obligations: ob, Score: a.Result.Score}, nil
			default:
				return Decision{Result: ResultError, ReasonCodes: []string{"opa_invalid_result_string"}, Obligations: Obligations{}}, nil
			}
		}
	}

	return Decision{Result: ResultError}, fmt.Errorf("pdp response unrecognized: %s", string(b))
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "pdp 5xx") {
		return true
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}