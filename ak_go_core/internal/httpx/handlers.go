package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"example.com/ak_go_core/internal/app/runs"
)

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

	var in CreateRunReq
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		WriteError(w, 400, "BadRequest", "invalid json", tid)
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
		TraceID:         tid,
		Source:          source,
	})
	if apiErr != nil {
		WriteError(w, apiErr.HTTPStatus, apiErr.Type, apiErr.Message, tid)
		return
	}

	WriteJSON(w, 202, CreateRunResp{
		RunID:   out.RunID,
		TraceID: out.TraceID,
		Status:  out.Status,
		Note:    out.Note,
	}, tid)
}

func (h *Handlers) GetRun(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())
	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
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
		WriteError(w, 404, "NotFound", "run not found", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}

func (h *Handlers) GetRunEvents(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())
	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
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
		WriteError(w, 404, "NotFound", "run not found", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}

func (h *Handlers) GetRunArtifacts(w http.ResponseWriter, req *http.Request) {
	tid := TraceIDFromContext(req.Context())
	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
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
		WriteError(w, 404, "NotFound", "run not found", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}
