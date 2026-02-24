package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type Config struct {
	WorkerID         string
	Poll             time.Duration
	ClaimStyle        postgres.ClaimStyle
	EvidenceMaxBytes int64
	EvidenceBaseDir  string

	IdleLogEvery int
}

// ✅ allowlist_key を必須入力にする（fail-closed）
type bodyFetcher interface {
	FetchBodyWithAllowlistKey(ctx context.Context, targetURL string, allowlistKey string) (*http.Response, error)
}

type Worker struct {
	store   *Store
	fetch   bodyFetcher
	logger  *log.Logger
	cfg     Config
	evStore EvidenceStore

	mb *ManifestBuilder

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

	mb := NewManifestBuilder(store.ManifestRepo)

	return &Worker{
		store:   store,
		fetch:   fetch,
		logger:  logger,
		cfg:     cfg,
		evStore: NewFSEvidenceStore(cfg.EvidenceBaseDir),
		mb:      mb,
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

	// ✅ v4 fixed: allowlist_key fail-closed
	allowKey := ""
	if in.AllowlistKey != nil {
		allowKey = strings.TrimSpace(*in.AllowlistKey)
	}
	if allowKey == "" {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "fetch_denied", "allowlist_key is required (fail-closed)")
		w.logger.Printf("denied: input_id=%d run_id=%s trace_id=%s url=%s reason=allowlist_key_required",
			in.ID, in.RunID, traceID, in.TargetURL)
		return nil
	}

	resp, ferr := w.fetch.FetchBodyWithAllowlistKey(ctx, in.TargetURL, allowKey)
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

	// --- derive final URL early (after redirects) ---
	finalURL := in.TargetURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	status := resp.StatusCode

	// content-type nullable
	ct := resp.Header.Get("Content-Type")
	var ctp *string
	if ct != "" {
		ctp = &ct
	}

	// 1) Save BODY evidence (stream -> file)
	bodyRel, bodySHA, bodyBytes, serr := w.evStore.SaveBlob(ctx, in.RunID, run.EvidenceKindFetchBody, "bin", resp.Body, w.cfg.EvidenceMaxBytes)
	if serr != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "evidence_store_failed", serr.Error())
		return serr
	}

	bodyAsset := run.EvidenceAsset{
		RunID:       in.RunID,
		TraceID:     traceID,
		Kind:        run.EvidenceKindFetchBody,
		ContentType: ctp,
		ByteSize:    bodyBytes,
		SHA256:      bodySHA,
		FinalURL:    finalURL,
		StoredPath:  bodyRel,
	}
	if err := w.store.EvidenceRepo.InsertEvidence(ctx, bodyAsset); err != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "evidence_insert_failed", err.Error())
		return err
	}

	// 2) Save META evidence (json -> file)
	meta := FetchMeta{
		Kind:          run.EvidenceKindFetchMeta,
		TargetURL:     in.TargetURL,
		FinalURL:      finalURL,
		StatusCode:    status,
		ContentType:   ct,
		BodyBytes:     bodyBytes,
		BodySHA256:    bodySHA,
		StoredBodyRel: bodyRel,
		FetchedAtUTC:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	metaJSON, _ := json.Marshal(meta)

	metaRel, metaSHA, metaBytes, merr := w.evStore.SaveBlob(ctx, in.RunID, run.EvidenceKindFetchMeta, "json", bytes.NewReader(metaJSON), w.cfg.EvidenceMaxBytes)
	if merr != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "meta_store_failed", merr.Error())
		return merr
	}

	metaCT := "application/json"
	metaCTP := &metaCT
	metaAsset := run.EvidenceAsset{
		RunID:       in.RunID,
		TraceID:     traceID,
		Kind:        run.EvidenceKindFetchMeta,
		ContentType: metaCTP,
		ByteSize:    metaBytes,
		SHA256:      metaSHA,
		FinalURL:    finalURL,
		StoredPath:  metaRel,
	}
	if err := w.store.EvidenceRepo.InsertEvidence(ctx, metaAsset); err != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "meta_insert_failed", err.Error())
		return err
	}

	// 3) Save HEADERS evidence (json -> file)
	// WARNING: headers can be large; we still cap by EvidenceMaxBytes.
	headersMap := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		lk := http.CanonicalHeaderKey(k)
		headersMap[lk] = append([]string(nil), v...)
	}
	headersJSON, _ := json.Marshal(headersMap)

	hdrRel, hdrSHA, hdrBytes, herr := w.evStore.SaveBlob(ctx, in.RunID, run.EvidenceKindFetchHeaders, "json", bytes.NewReader(headersJSON), w.cfg.EvidenceMaxBytes)
	if herr != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "headers_store_failed", herr.Error())
		return herr
	}

	hdrCT := "application/json"
	hdrCTP := &hdrCT
	headersAsset := run.EvidenceAsset{
		RunID:       in.RunID,
		TraceID:     traceID,
		Kind:        run.EvidenceKindFetchHeaders,
		ContentType: hdrCTP,
		ByteSize:    hdrBytes,
		SHA256:      hdrSHA,
		FinalURL:    finalURL,
		StoredPath:  hdrRel,
	}
	if err := w.store.EvidenceRepo.InsertEvidence(ctx, headersAsset); err != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "headers_insert_failed", err.Error())
		return err
	}

	// v4.5: manifest build & complete (DB links set is SoT)
	assetsForManifest := []run.EvidenceAsset{bodyAsset, metaAsset, headersAsset}

	manifest, mhash, berr := w.mb.BuildAndComplete(ctx, in.RunID, traceID, assetsForManifest)
	if berr != nil {
		_ = w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, "manifest_failed", berr.Error())
		return berr
	}

	// 4) done / retry decision
	if status >= 200 && status < 300 {
		if err := w.store.ClaimRepo.MarkDone(ctx, in.ID, w.cfg.WorkerID); err != nil {
			return err
		}
		w.logger.Printf("done: input_id=%d run_id=%s trace_id=%s status=%d body_bytes=%d body_sha=%s manifest_id=%s manifest_hash=%s",
			in.ID, in.RunID, traceID, status, bodyBytes, bodySHA, manifest.ManifestID, mhash)
		return nil
	}

	code := fmt.Sprintf("http_%d", status)
	msg := fmt.Sprintf("non-2xx status=%d final_url=%s", status, finalURL)
	if err := w.store.ClaimRepo.MarkRetry(ctx, in.ID, w.cfg.WorkerID, code, msg); err != nil {
		return err
	}

	if status >= 500 || status == 429 || status == 408 {
		w.logger.Printf("retryable_http: input_id=%d run_id=%s trace_id=%s status=%d url=%s manifest_id=%s manifest_hash=%s",
			in.ID, in.RunID, traceID, status, finalURL, manifest.ManifestID, mhash)
		return nil
	}

	w.logger.Printf("terminal_http: input_id=%d run_id=%s trace_id=%s status=%d url=%s manifest_id=%s manifest_hash=%s",
		in.ID, in.RunID, traceID, status, finalURL, manifest.ManifestID, mhash)
	return nil
}