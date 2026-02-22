package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
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
}

type StartFetchRunOutput struct {
	RunID        string
	TraceID      string
	Status       string // done/failed
	ErrorCode    *string
	ErrorMessage *string
}

// StartFetchRunUseCase はフェッチ実行のオーケストレーションを行う
type StartFetchRunUseCase struct {
	Fetcher      Fetcher // ★ PISAGFetcher 注入（allow/denyの真実はここ）
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

	// ✅ validateURL は「https必須」だけ
	// host/port/path/allowlist/redirect/ip/tls 等は PISAG(RequestGuard) を唯一の真実に寄せる
	if err := validateURL(in.TargetURL); err != nil {
		return StartFetchRunOutput{}, err
	}

	runID := uuid.New().String()
	traceID := uuid.New().String()

	r := run.Run{
		RunID:           runID,
		ProjectID:       in.ProjectID,
		TraceID:         traceID,
		PipelineVersion: in.PipelineVersion,
		Status:          run.StatusRunning,
	}

	// 1) runs 作成（ak_worker）
	_, err := uc.RunRepo.Create(ctx, r)
	if err != nil {
		return StartFetchRunOutput{}, err
	}

	// 2) run_inputs
	headersJSON := []byte(`{}`)
	_ = uc.RunInputRepo.Insert(ctx, run.RunInput{
		RunID:        runID,
		SourceID:     in.SourceID,
		TargetURL:    in.TargetURL,
		Method:       "GET",
		HeadersJSON:  headersJSON,
		AllowlistKey: in.AllowlistKey,
	})

	_ = uc.appendEvent(ctx, runID, traceID, "fetch_started", "fetch", "ok", nil, map[string]any{
		"target_url": in.TargetURL,
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
	})

	// 3) フェッチ（PISAGが唯一の真実）
	res, ferr := uc.Fetcher.Fetch(ctx, in.TargetURL)
	if ferr != nil {
		code := "fetch_failed"
		msg := ferr.Error()
		if errors.Is(ferr, ErrDenied) {
			code = "fetch_denied"
		}

		_ = uc.appendEvent(ctx, runID, traceID, "fetch_failed", "fetch", "failed", &msg, map[string]any{
			"error_code": code,
		})

		_ = uc.RunRepo.MarkFailed(ctx, runID, code, msg)

		// 重要: infra以外は panic しない。UseCase は成功応答(=failed)で返す。
		return StartFetchRunOutput{
			RunID:        runID,
			TraceID:      traceID,
			Status:       "failed",
			ErrorCode:    &code,
			ErrorMessage: &msg,
		}, nil
	}

	_ = uc.appendEvent(ctx, runID, traceID, "fetch_done", "fetch", "ok", nil, map[string]any{
		"final_url":      res.FinalURL,
		"status_code":    res.StatusCode,
		"content_type":   res.ContentType,
		"body_size":      res.BodySize,
		"fetched_at_utc": time.Now().UTC().Format(time.RFC3339Nano),
	})

	_ = uc.RunRepo.MarkDone(ctx, runID)

	return StartFetchRunOutput{
		RunID:   runID,
		TraceID: traceID,
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

// validateURL は「https必須」だけ。
// host/port/path/allowlist/redirect/ip/tls の制約は PISAG(RequestGuard) に一本化する。
func validateURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("only https is allowed")
	}
	return nil
}