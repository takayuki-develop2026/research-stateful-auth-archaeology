package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"example.com/ak_go_core/internal/app/runs"
	"bytes"
 	"io"
  	"log"
)

/*
P0 contract (v3):
- HTTP should expose:
  - state: internal progress state (queued/running/done/...)
  - status: public 2-value status (review_required/failed/omitted)
  - result: pending/success/failed
- Always include trace_id in response header via TraceMiddleware.
*/

type Handlers struct {
	runs *runs.Service
}

func NewHandlers(runsSvc *runs.Service) *Handlers {
	return &Handlers{runs: runsSvc}
}

func (h *Handlers) PostAnalyze(w http.ResponseWriter, req *http.Request) {
	h.postCreateRun(w, req, "analyze.create")
}

func (h *Handlers) PostRuns(w http.ResponseWriter, req *http.Request) {
	h.postCreateRun(w, req, "runs.create")
}

func (h *Handlers) postCreateRun(w http.ResponseWriter, req *http.Request, source string) {
	tid := TraceIDFromContext(req.Context())

	// 1. Method Guard
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed", tid)
		return
	}

	// 2. Body Safety (1MB)
	req.Body = http.MaxBytesReader(w, req.Body, 1<<20)
	defer req.Body.Close()

	// 3. Decode with Diagnostic Logging
	var in CreateRunReq
	var bodyBuf bytes.Buffer
	// Bodyを読みながら同時にBufにも書き込む（エラー時の調査用）
	tee := io.TeeReader(req.Body, &bodyBuf)

	dec := json.NewDecoder(tee)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&in); err != nil {
		raw := bodyBuf.Bytes()
		head := raw
		if len(head) > 1024 {
			head = head[:1024]
		}
		// 徹底した調査ログ: エラーの型、Content-Type、生のBody（先頭）を記録
		log.Printf("[runs.create] tid=%s ERROR: decode_failed err=%T %v ct=%q cl=%d head=%q head_hex=%x",
			tid, err, err, req.Header.Get("Content-Type"), req.ContentLength, string(head), head)

		WriteError(w, http.StatusBadRequest, "BadRequest", "invalid json: "+err.Error(), tid)
		return
	}

	// 4. Trailing Garbage Check (正確な検知)
	// io.EOF 以外が返ってきたら、JSONの後に余計なデータがある
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		log.Printf("[runs.create] tid=%s ERROR: trailing_garbage err=%v", tid, err)
		WriteError(w, http.StatusBadRequest, "BadRequest", "invalid json: trailing garbage", tid)
		return
	}

	// 5. Logic Execution
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	out, apiErr := h.runs.CreateRun(ctx, runs.CreateRunInput{
		ProjectID:       in.ProjectID,
		PolicyVersion:   in.PolicyVersion,
		PipelineVersion: in.PipelineVersion,
		Mode:            in.Mode,
		RequestKey:      ReadIdempotencyKey(req),
		TraceID:         tid,
		Source:          source,
	})

	if apiErr != nil {
		// サービス層のエラーも詳細にログ
		log.Printf("[runs.create] tid=%s ERROR: service_error type=%s msg=%s", tid, apiErr.Type, apiErr.Message)
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}

	// 6. Response
	WriteJSON(w, http.StatusAccepted, CreateRunResp{
		RunID:   out.RunID,
		TraceID: out.TraceID,
		State:   out.State,
		Status:  out.Status,
		Result:  out.Result,
		Note:    out.Note,
	}, tid)
}

func (h *Handlers) GetRun(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())

	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed", tid)
		return
	}

	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, http.StatusBadRequest, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	out, ok, apiErr := h.runs.GetRun(ctx, runID)
	if apiErr != nil {
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}
	if !ok {
		WriteError(w, http.StatusNotFound, "NotFound", "run not found", tid)
		return
	}

	// out already contains state/status/result (normalized in runs.Service)
	WriteJSON(w, http.StatusOK, out, tid)
}

func (h *Handlers) GetRunEvents(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())

	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed", tid)
		return
	}

	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, http.StatusBadRequest, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	out, ok, apiErr := h.runs.GetRunEvents(ctx, runID)
	if apiErr != nil {
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}
	if !ok {
		WriteError(w, http.StatusNotFound, "NotFound", "run not found", tid)
		return
	}

	WriteJSON(w, http.StatusOK, out, tid)
}

func (h *Handlers) GetRunArtifacts(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())

	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed", tid)
		return
	}

	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, http.StatusBadRequest, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	out, ok, apiErr := h.runs.GetRunArtifacts(ctx, runID)
	if apiErr != nil {
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}
	if !ok {
		WriteError(w, http.StatusNotFound, "NotFound", "run not found", tid)
		return
	}

	WriteJSON(w, http.StatusOK, out, tid)
}