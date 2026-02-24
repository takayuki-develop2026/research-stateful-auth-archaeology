package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type Config struct {
	WorkerID          string
	Poll              time.Duration
	ClaimStyle         postgres.ClaimStyle
	EvidenceMaxBytes   int64
	EvidenceBaseDir    string
	IdleLogEvery       int
}

type bodyFetcher interface {
	FetchBody(ctx context.Context, targetURL string) (*http.Response, error)
}

type Worker struct {
	store   *Store
	fetch   bodyFetcher
	logger  *log.Logger
	cfg     Config
	evStore EvidenceStore

	idleCount int
}

func NewWorker(store *Store, fetch bodyFetcher, logger *log.Logger, cfg Config) *Worker {
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	if cfg.ClaimStyle == "" {
		cfg.ClaimStyle = postgres.ClaimStyleCTE
	}
	if cfg.EvidenceMaxBytes <= 0 {
		cfg.EvidenceMaxBytes = 5 << 20
	}
	if cfg.EvidenceBaseDir == "" {
		cfg.EvidenceBaseDir = "./var/evidence"
	}
	if cfg.IdleLogEvery <= 0 {
		cfg.IdleLogEvery = 10
	}
	return &Worker{
		store:   store,
		fetch:   fetch,
		logger:  logger,
		cfg:     cfg,
		evStore: NewFSEvidenceStore(cfg.EvidenceBaseDir),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Printf("worker started: id=%s poll=%s claim_style=%s idle_log_every=%d",
		w.cfg.WorkerID, w.cfg.Poll, w.cfg.ClaimStyle, w.cfg.IdleLogEvery)

	t := time.NewTicker(w.cfg.Poll)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := w.tick(ctx); err != nil {
				w.logger.Printf("tick error: %v", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	in, err := w.store.ClaimRepo.ClaimNext(ctx, w.cfg.WorkerID, w.cfg.ClaimStyle)
	if err != nil {
		return err
	}

	if in == nil {
		w.idleCount++
		if w.idleCount%w.cfg.IdleLogEvery == 0 {
			w.logger.Printf("idle: no pending input")
		}
		return nil
	}
	w.idleCount = 0

	traceID := in.TraceID
	if traceID == "" {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "trace_missing", "trace_id was empty in claimed input")
		w.logger.Printf("trace_missing: input_id=%d run_id=%s url=%s", in.ID, in.RunID, in.TargetURL)
		return nil
	}

	resp, ferr := w.fetch.FetchBody(ctx, in.TargetURL)
	if ferr != nil {
		if errors.Is(ferr, usecase.ErrDenied) {
			_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "fetch_denied", ferr.Error())
			w.logger.Printf("denied: input_id=%d run_id=%s trace_id=%s url=%s reason=%s",
				in.ID, in.RunID, traceID, in.TargetURL, ferr.Error())
			return nil
		}

		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "fetch_failed", ferr.Error())
		w.logger.Printf("fetch_failed: input_id=%d run_id=%s trace_id=%s url=%s err=%s",
			in.ID, in.RunID, traceID, in.TargetURL, ferr.Error())
		return nil
	}
	defer resp.Body.Close()

	storedRel, sha, size, serr := w.evStore.SaveFetchBody(ctx, in.RunID, resp.Body, w.cfg.EvidenceMaxBytes)
	if serr != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "evidence_store_failed", serr.Error())
		return serr
	}

	ct := resp.Header.Get("Content-Type")
	var ctp *string
	if ct != "" {
		ctp = &ct
	}

	finalURL := in.TargetURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	ev := run.EvidenceAsset{
		RunID:       in.RunID,
		TraceID:     traceID,
		Kind:        "fetch_body",
		ContentType: ctp,
		ByteSize:    size,
		SHA256:      sha,
		FinalURL:    finalURL,
		StoredPath:  storedRel,
	}
	if err := w.store.EvidenceRepo.InsertEvidence(ctx, ev); err != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "evidence_insert_failed", err.Error())
		return err
	}

	status := resp.StatusCode

	if status >= 200 && status < 300 {
		if err := w.store.ClaimRepo.MarkDone(ctx, in.ID, w.cfg.WorkerID); err != nil {
			return err
		}
		w.logger.Printf("done: input_id=%d run_id=%s trace_id=%s status=%d bytes=%d sha=%s",
			in.ID, in.RunID, traceID, status, size, sha)
		return nil
	}

	code := fmt.Sprintf("http_%d", status)
	msg := fmt.Sprintf("non-2xx status=%d final_url=%s", status, finalURL)
	if err := w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, code, msg); err != nil {
		return err
	}

	if status >= 500 || status == 429 || status == 408 {
		w.logger.Printf("retryable_http: input_id=%d run_id=%s trace_id=%s status=%d bytes=%d sha=%s url=%s",
			in.ID, in.RunID, traceID, status, size, sha, finalURL)
		return nil
	}

	w.logger.Printf("terminal_http: input_id=%d run_id=%s trace_id=%s status=%d url=%s",
		in.ID, in.RunID, traceID, status, finalURL)
	return nil
}