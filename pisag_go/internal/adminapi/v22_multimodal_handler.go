package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type V22MultimodalHandler struct {
	CreateTaskUC    *usecase.CreateMultimodalTaskUseCase
	AttachInputsUC  *usecase.AttachMultimodalTaskInputsUseCase
	ExecuteOCRUC    *usecase.ExecuteV22OCRTaskUseCase
	ExecuteVisionUC *usecase.ExecuteV22VisionTaskUseCase

	TaskRepo        run.MultimodalTaskRepository
	ResultRepo      run.MultimodalResultRepository
	ReviewQueueRepo run.MultimodalReviewQueueRepository
	NormalizedRepo  run.NormalizedMultimodalResultRepository
	DownstreamRepo  run.MultimodalDownstreamHandoffRepository
}

type v22ErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

type createV22TaskRequest struct {
	ProjectID                 string `json:"project_id"`
	TraceID                   string `json:"trace_id"`
	RunID                     string `json:"run_id"`
	TaskType                  string `json:"task_type"`
	PipelineVersion           string `json:"pipeline_version"`
	PolicyVersionStr          string `json:"policy_version_str"`
	InputHash                 string `json:"input_hash"`
	RouterPlanEvidenceAssetID int64  `json:"router_plan_evidence_asset_id"`
	OptionsEvidenceAssetID    int64  `json:"options_evidence_asset_id"`
	Inputs                    []struct {
		EvidenceID int64  `json:"evidence_id"`
		InputRole  string `json:"input_role"`
		Seq        int    `json:"seq"`
	} `json:"inputs"`
}

type executeV22TaskRequest struct {
	ProjectID       string `json:"project_id"`
	DestinationKind string `json:"destination_kind"`
}

func (h *V22MultimodalHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.CreateTaskUC == nil || h.AttachInputsUC == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "task create wiring is incomplete", traceID)
		return
	}

	var req createV22TaskRequest
	if err := v22DecodeJSON(r, &req); err != nil {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	createOut, err := h.CreateTaskUC.Handle(r.Context(), usecase.CreateMultimodalTaskInput{
		ProjectID:                 req.ProjectID,
		TraceID:                   req.TraceID,
		RunID:                     req.RunID,
		TaskType:                  run.MultimodalTaskType(req.TaskType),
		PipelineVersion:           req.PipelineVersion,
		PolicyVersionStr:          req.PolicyVersionStr,
		InputHash:                 req.InputHash,
		RouterPlanEvidenceAssetID: req.RouterPlanEvidenceAssetID,
		OptionsEvidenceAssetID:    req.OptionsEvidenceAssetID,
	})
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "create_task_failed", err.Error(), traceID)
		return
	}

	var inputs []run.AttachMultimodalTaskInputInput
	for _, in := range req.Inputs {
		inputs = append(inputs, run.AttachMultimodalTaskInputInput{
			ProjectID:  req.ProjectID,
			TaskID:     createOut.Task.ID,
			EvidenceID: in.EvidenceID,
			InputRole:  run.MultimodalInputRole(in.InputRole),
			Seq:        in.Seq,
		})
	}

	attachOut, err := h.AttachInputsUC.Handle(r.Context(), usecase.AttachMultimodalTaskInputsInput{
		ProjectID: req.ProjectID,
		TaskID:    createOut.Task.ID,
		Inputs:    inputs,
	})
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "attach_task_inputs_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusCreated, traceID, map[string]any{
		"task":   createOut.Task,
		"inputs": attachOut.Inputs,
	})
}

func (h *V22MultimodalHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.TaskRepo == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "task repo is nil", traceID)
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if projectID == "" || runID == "" {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", "project_id and run_id are required", traceID)
		return
	}

	items, err := h.TaskRepo.ListByRunID(r.Context(), projectID, runID)
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "list_tasks_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *V22MultimodalHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.TaskRepo == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "task repo is nil", traceID)
		return
	}

	id, ok := v22ExtractIDFromPath(r.URL.Path, "/v22/multimodal/tasks/")
	if !ok {
		v22WriteError(w, http.StatusBadRequest, "invalid_path", "invalid task path", traceID)
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", "project_id is required", traceID)
		return
	}

	task, err := h.TaskRepo.FindByID(r.Context(), id)
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "get_task_failed", err.Error(), traceID)
		return
	}
	if task.ProjectID != projectID {
		v22WriteError(w, http.StatusForbidden, "project_mismatch", "task project mismatch", traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"task": task,
	})
}

func (h *V22MultimodalHandler) ListResults(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.ResultRepo == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "result repo is nil", traceID)
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if projectID == "" || runID == "" {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", "project_id and run_id are required", traceID)
		return
	}

	items, err := h.ResultRepo.ListByRunID(r.Context(), projectID, runID)
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "list_results_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *V22MultimodalHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.ResultRepo == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "result repo is nil", traceID)
		return
	}

	id, ok := v22ExtractIDFromPath(r.URL.Path, "/v22/multimodal/results/")
	if !ok {
		v22WriteError(w, http.StatusBadRequest, "invalid_path", "invalid result path", traceID)
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", "project_id is required", traceID)
		return
	}

	result, err := h.ResultRepo.FindByID(r.Context(), id)
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "get_result_failed", err.Error(), traceID)
		return
	}
	if result.ProjectID != projectID {
		v22WriteError(w, http.StatusForbidden, "project_mismatch", "result project mismatch", traceID)
		return
	}

	var normalized any
	if h.NormalizedRepo != nil {
		if v, err := h.NormalizedRepo.FindByResultID(r.Context(), projectID, result.ID); err == nil {
			normalized = v
		}
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"result":            result,
		"normalized_result": normalized,
	})
}

func (h *V22MultimodalHandler) ListReviewQueue(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.ReviewQueueRepo == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "review queue repo is nil", traceID)
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if projectID == "" {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", "project_id is required", traceID)
		return
	}

	var (
		items []run.MultimodalReviewQueueItem
		err   error
	)
	if runID != "" {
		items, err = h.ReviewQueueRepo.ListByRunID(r.Context(), projectID, runID)
	} else {
		items, err = h.ReviewQueueRepo.ListPendingByProjectID(r.Context(), projectID)
	}
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "list_review_queue_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *V22MultimodalHandler) ExecuteOCRTask(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.ExecuteOCRUC == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "execute ocr usecase is nil", traceID)
		return
	}

	taskID, ok := v22ExtractNestedIDFromPath(r.URL.Path, "/v22/multimodal/tasks/", "/execute/ocr")
	if !ok {
		v22WriteError(w, http.StatusBadRequest, "invalid_path", "invalid ocr execute path", traceID)
		return
	}

	var req executeV22TaskRequest
	if err := v22DecodeJSON(r, &req); err != nil {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.ExecuteOCRUC.Handle(r.Context(), usecase.ExecuteV22OCRTaskInput{
		ProjectID:       req.ProjectID,
		TaskID:          taskID,
		DestinationKind: req.DestinationKind,
	})
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "execute_ocr_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"task":              out.Task,
		"result":            out.Result,
		"normalized_result": out.NormalizedResult,
	})
}

func (h *V22MultimodalHandler) ExecuteVisionTask(w http.ResponseWriter, r *http.Request) {
	traceID := v22EnsureTraceID(r)
	if h.ExecuteVisionUC == nil {
		v22WriteError(w, http.StatusInternalServerError, "internal_error", "execute vision usecase is nil", traceID)
		return
	}

	taskID, ok := v22ExtractNestedIDFromPath(r.URL.Path, "/v22/multimodal/tasks/", "/execute/vision")
	if !ok {
		v22WriteError(w, http.StatusBadRequest, "invalid_path", "invalid vision execute path", traceID)
		return
	}

	var req executeV22TaskRequest
	if err := v22DecodeJSON(r, &req); err != nil {
		v22WriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.ExecuteVisionUC.Handle(r.Context(), usecase.ExecuteV22VisionTaskInput{
		ProjectID:       req.ProjectID,
		TaskID:          taskID,
		DestinationKind: req.DestinationKind,
	})
	if err != nil {
		v22WriteError(w, http.StatusBadRequest, "execute_vision_failed", err.Error(), traceID)
		return
	}

	v22WriteJSON(w, http.StatusOK, traceID, map[string]any{
		"task":              out.Task,
		"result":            out.Result,
		"normalized_result": out.NormalizedResult,
	})
}

func v22DecodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func v22WriteJSON(w http.ResponseWriter, status int, traceID string, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Trace-Id", traceID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func v22WriteError(w http.ResponseWriter, status int, typ, msg, traceID string) {
	var env v22ErrorEnvelope
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	v22WriteJSON(w, status, traceID, env)
}

func v22EnsureTraceID(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Trace-Id"))
	if v != "" {
		return v
	}
	return "trace_v22_http"
}

func v22ExtractIDFromPath(path, prefix string) (int64, bool) {
	s := strings.TrimPrefix(path, prefix)
	s = strings.Trim(s, "/")
	if s == "" || strings.Contains(s, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func v22ExtractNestedIDFromPath(path, prefix, suffix string) (int64, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	s = strings.Trim(s, "/")
	if s == "" || strings.Contains(s, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func v22MethodNotAllowed(w http.ResponseWriter, traceID string, got string, allow ...string) {
	v22WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", fmt.Sprintf("method=%s allow=%v", got, allow), traceID)
}

type v22TaskQueryContextKey string

const v22TaskQueryContextKeyProjectID v22TaskQueryContextKey = "project_id"

func v22WithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, v22TaskQueryContextKeyProjectID, projectID)
}
