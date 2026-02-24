package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"example.com/pisag_go/run"
	"github.com/google/uuid"
)

type StartFetchRunInput struct {
	ProjectID       string
	TargetURL       string
	SourceID        *string
	PipelineVersion string
	AllowlistKey    *string

	// debug/local only: enqueue後に同期fetchまで実行したい場合だけtrue
	ImmediateFetch bool

	// v4.2: 同一目的のrunを再利用する（推奨: デフォルト true）
	// nil => true
	ReuseRun *bool
}

type StartFetchRunOutput struct {
	RunID        string
	TraceID      string
	Status       string // enqueued/done/failed
	ErrorCode    *string
	ErrorMessage *string
}

type StartFetchRunUseCase struct {
	Fetcher      Fetcher
	RunRepo      run.RunRepo
	RunInputRepo run.RunInputRepo
	RunEventRepo run.RunEventRepo
}

func (uc *StartFetchRunUseCase) Handle(ctx context.Context, in StartFetchRunInput) (StartFetchRunOutput, error) {
	if in.ProjectID == "" {
		return StartFetchRunOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.TargetURL) == "" {
		return StartFetchRunOutput{}, errors.New("target_url is required")
	}
	if in.PipelineVersion == "" {
		in.PipelineVersion = "v4.1"
	}

	// ここは https/host 必須だけ。詳細ガードはPISAGに一任
	if err := validateURL(in.TargetURL); err != nil {
		return StartFetchRunOutput{}, err
	}

	method := "GET"
	allow := ""
	if in.AllowlistKey != nil {
		allow = strings.TrimSpace(*in.AllowlistKey)
	}

	// enqueue/run key用にURL正規化（PISAGガードの代替ではない）
	nurl, nerr := normalizeURLForEnqueueKey(in.TargetURL)
	if nerr != nil {
		return StartFetchRunOutput{}, nerr
	}

	// v4.2: run_key（同一目的なら同一run）
	runKey := hashHex("run|" + in.PipelineVersion + "|" + method + "|" + allow + "|" + nurl)

	// ✅ ReuseRun デフォルト true
	reuse := boolDefaultTrue(in.ReuseRun)

	// run作成 or 再利用
	var r run.Run
	var reused bool

	if reuse {
		rr, found, err := uc.RunRepo.CreateOrGetByRunKey(ctx, in.ProjectID, runKey, func() run.Run {
			runID := uuid.New().String()
			traceID := uuid.New().String()
			return run.Run{
				RunID:           runID,
				ProjectID:       in.ProjectID,
				TraceID:         traceID,
				PipelineVersion: in.PipelineVersion,
				Status:          run.StatusRunning,
				RunKey:          ptr(runKey),
			}
		})
		if err != nil {
			return StartFetchRunOutput{}, err
		}
		r = rr
		reused = found
	} else {
		runID := uuid.New().String()
		traceID := uuid.New().String()
		r = run.Run{
			RunID:           runID,
			ProjectID:       in.ProjectID,
			TraceID:         traceID,
			PipelineVersion: in.PipelineVersion,
			Status:          run.StatusRunning,
			RunKey:          nil, // ✅ reuseしないrunにはrun_keyを持たせない
		}
		if _, err := uc.RunRepo.Create(ctx, r); err != nil {
			return StartFetchRunOutput{}, err
		}
		reused = false
	}

	// enqueue_key（同一run内で同一入力を1回に潰す）
	enqueueKey := hashHex("fetch|" + method + "|" + allow + "|" + nurl)

	headersJSON := []byte(`{}`)
	if err := uc.RunInputRepo.Insert(ctx, run.RunInput{
		RunID:        r.RunID,
		SourceID:     in.SourceID,
		TargetURL:    in.TargetURL, // raw保持（証拠）
		Method:       method,
		HeadersJSON:  headersJSON,
		AllowlistKey: in.AllowlistKey,
		EnqueueKey:   enqueueKey,
	}); err != nil {
		return StartFetchRunOutput{}, err
	}

	_ = uc.appendEvent(ctx, r.RunID, r.TraceID, "fetch_enqueued", "fetch", "ok", nil, map[string]any{
		"target_url":       in.TargetURL,
		"normalized_url":   nurl,
		"enqueue_key":      enqueueKey,
		"run_key":          runKey,
		"run_reused":       reused,
		"reuse_run":        reuse,
		"pipeline_version": in.PipelineVersion,
		"ts":               time.Now().UTC().Format(time.RFC3339Nano),
	})

	// worker主体ならここで終了（正道）
	if !in.ImmediateFetch {
		return StartFetchRunOutput{
			RunID:   r.RunID,
			TraceID: r.TraceID,
			Status:  "enqueued",
		}, nil
	}

	// debug/local only: immediate fetch
	res, ferr := uc.Fetcher.Fetch(ctx, in.TargetURL)
	if ferr != nil {
		code := "fetch_failed"
		msg := ferr.Error()
		if errors.Is(ferr, ErrDenied) {
			code = "fetch_denied"
		}
		_ = uc.appendEvent(ctx, r.RunID, r.TraceID, "fetch_failed", "fetch", "failed", &msg, map[string]any{
			"error_code": code,
		})
		_ = uc.RunRepo.MarkFailed(ctx, r.RunID, code, msg)
		return StartFetchRunOutput{
			RunID:        r.RunID,
			TraceID:      r.TraceID,
			Status:       "failed",
			ErrorCode:    &code,
			ErrorMessage: &msg,
		}, nil
	}

	_ = uc.appendEvent(ctx, r.RunID, r.TraceID, "fetch_done", "fetch", "ok", nil, map[string]any{
		"final_url":      res.FinalURL,
		"status_code":    res.StatusCode,
		"content_type":   res.ContentType,
		"body_size":      res.BodySize,
		"fetched_at_utc": time.Now().UTC().Format(time.RFC3339Nano),
	})

	_ = uc.RunRepo.MarkDone(ctx, r.RunID)

	return StartFetchRunOutput{
		RunID:   r.RunID,
		TraceID: r.TraceID,
		Status:  "done",
	}, nil
}

func (uc *StartFetchRunUseCase) appendEvent(
	ctx context.Context,
	runID, traceID, name, step, status string,
	message *string,
	data map[string]any,
) error {
	b, _ := json.Marshal(data)
	return uc.RunEventRepo.Append(ctx, run.RunEvent{
		RunID:     runID,
		TraceID:   traceID,
		EventName: name,
		Step:      step,
		Status:    status,
		Message:   message,
		DataJSON:  b,
	})
}

func validateURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("only https is allowed")
	}
	if u.Host == "" {
		return errors.New("host is required")
	}
	return nil
}

// normalizeURLForEnqueueKey: enqueue/run key生成用（PISAGガードの代替ではない）
func normalizeURLForEnqueueKey(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", errors.New("only https is allowed")
	}
	if u.Host == "" {
		return "", errors.New("host is required")
	}

	u.Fragment = ""

	// host normalize
	host := strings.ToLower(u.Host)
	if h, p, e := net.SplitHostPort(host); e == nil {
		if p == "443" {
			host = h
		} else {
			host = net.JoinHostPort(h, p)
		}
	}
	u.Host = host

	// path normalize
	if u.Path == "" {
		u.Path = "/"
	} else {
		cp := path.Clean(u.Path)
		if cp == "." {
			cp = "/"
		}
		if cp != "/" {
			cp = strings.TrimRight(cp, "/")
			if cp == "" {
				cp = "/"
			}
		}
		u.Path = cp
	}

	// query normalize
	if u.RawQuery != "" {
		q := u.Query()
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var b strings.Builder
		first := true
		for _, k := range keys {
			vals := q[k]
			sort.Strings(vals)
			for _, v := range vals {
				if !first {
					b.WriteByte('&')
				}
				first = false
				b.WriteString(url.QueryEscape(k))
				b.WriteByte('=')
				b.WriteString(url.QueryEscape(v))
			}
		}
		u.RawQuery = b.String()
	}

	out := u.Scheme + "://" + u.Host + u.Path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, nil
}

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func ptr(s string) *string { return &s }

// nil => true
func boolDefaultTrue(p *bool) bool {
	if p == nil {
		return true
	}
	return *p
}

// NormalizeURLForEnqueueKey_ForTest exposes the internal normalization for tests.
func NormalizeURLForEnqueueKey_ForTest(raw string) (string, error) {
	return normalizeURLForEnqueueKey(raw)
}