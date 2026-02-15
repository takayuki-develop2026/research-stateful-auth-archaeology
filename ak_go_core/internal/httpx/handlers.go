package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"example.com/ak_go_core/internal/app/runs"
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

	// method guard (defensive)
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed", tid)
		return
	}

	// body safety (1MB; adjust if you expect bigger)
	req.Body = http.MaxBytesReader(w, req.Body, 1<<20)
	defer req.Body.Close()

	var in CreateRunReq
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "BadRequest", "invalid json", tid)
		return
	}

	// Optional: reject trailing garbage (e.g. "{}{}")
	if dec.More() {
		WriteError(w, http.StatusBadRequest, "BadRequest", "invalid json", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	out, apiErr := h.runs.CreateRun(ctx, runs.CreateRunInput{
		ProjectID:       in.ProjectID,
		PolicyVersion:   in.PolicyVersion,
		PipelineVersion: in.PipelineVersion,
		Mode:            in.Mode,
		RequestKey:      ReadIdempotencyKey(req),
		TraceID:         tid, // TraceMiddleware generated (or upstream)
		Source:          source,
	})
	if apiErr != nil {
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}

	// P0: return state/status/result (do NOT overload "status" with blocked)
	WriteJSON(w, http.StatusAccepted, CreateRunResp{
		RunID:   out.RunID,
		TraceID: out.TraceID,
		State:   out.State,
		Status:  out.Status, // review_required / failed / "" => omitted
		Result:  out.Result, // pending/success/failed
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