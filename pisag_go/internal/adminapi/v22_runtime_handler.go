package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type V22RuntimeHandler struct {
	RouteTask               *usecase.RouteV22ModelTaskUseCase
	RegisterRequestEvidence *usecase.RegisterRuntimeRequestEvidenceUseCase
	CreateTask              *usecase.CreateMultimodalTaskUseCase
	AttachTaskInputs        *usecase.AttachMultimodalTaskInputsUseCase
	GetRunDetail            *usecase.GetRuntimeRunDetailUseCase
}

func (h *V22RuntimeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/catalog", h.handleCatalog)
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/runs", h.handleRuns)
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/runs/", h.handleRunByID)
}

func (h *V22RuntimeHandler) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONBody(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method_not_allowed",
		})
		return
	}

	catalog := run.DefaultV221EngineCatalog()

	writeJSONBody(w, http.StatusOK, map[string]any{
		"preprocess": defsToJSON(catalog.ListByCapability(run.EngineCapabilityPreprocess)),
		"ocr":        defsToJSON(catalog.ListByCapability(run.EngineCapabilityOCR)),
		"docparse":   defsToJSON(catalog.ListByCapability(run.EngineCapabilityDocParse)),
		"embedding":  defsToJSON(catalog.ListByCapability(run.EngineCapabilityEmbedding)),
		"vision":     defsToJSON(catalog.ListByCapability(run.EngineCapabilityVision)),
		"llm":        defsToJSON(catalog.ListByCapability(run.EngineCapabilityLLM)),
	})
}

func (h *V22RuntimeHandler) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreateRun(w, r)
	case http.MethodGet:
		writeJSONBody(w, http.StatusOK, map[string]any{
			"items": []any{},
		})
	default:
		writeJSONBody(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method_not_allowed",
		})
	}
}

func (h *V22RuntimeHandler) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if h.RouteTask == nil || h.RegisterRequestEvidence == nil || h.CreateTask == nil || h.AttachTaskInputs == nil {
		writeJSONBody(w, http.StatusInternalServerError, map[string]any{
			"error": "runtime_handler_not_configured",
		})
		return
	}

	var req run.RuntimeCreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_json",
			"message": err.Error(),
		})
		return
	}

	traceID := GenerateTraceIDOrDefault(req.TraceID)
	runID := GenerateRunID(req.ProjectID, req.TaskType)

	selection := BuildSelectionFromRequest(req)

	routeOut, err := h.RouteTask.Handle(r.Context(), usecase.RouteV22ModelTaskInput{
		ProjectID:       req.ProjectID,
		TaskType:        req.TaskType,
		Preset:          req.Preset,
		Selection:       &selection,
		PipelineVersion: req.PipelineVersion,
		PolicyVersion:   req.PolicyVersionStr,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "route_task_failed",
			"message": err.Error(),
		})
		return
	}

	inputRefs := BuildInputRefsFromRequest(req)

	inputHash, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        req.ProjectID,
		TaskType:         req.TaskType,
		PipelineVersion:  req.PipelineVersion,
		PolicyVersionStr: req.PolicyVersionStr,
		OptionsCanonical: routeOut.OptionsCanonicalJSON,
		Inputs:           inputRefs,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "build_input_hash_failed",
			"message": err.Error(),
		})
		return
	}

	optionsSHA := BuildOptionsSHA256(routeOut.OptionsCanonicalJSON)
	routeSHA := BuildRoutePlanSHA256(routeOut.RoutePlanCanonicalJSON)

	evOut, err := h.RegisterRequestEvidence.Handle(r.Context(), usecase.RegisterRuntimeRequestEvidenceInput{
		ProjectID:               req.ProjectID,
		TraceID:                 traceID,
		OptionsCanonicalJSON:    CompactJSON(routeOut.OptionsCanonicalJSON),
		RoutePlanCanonicalJSON:  CompactJSON(routeOut.RoutePlanCanonicalJSON),
		OptionsSHA256:           optionsSHA,
		RoutePlanSHA256:         routeSHA,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "register_request_evidence_failed",
			"message": err.Error(),
		})
		return
	}

	taskOut, err := h.CreateTask.Handle(r.Context(), usecase.CreateMultimodalTaskInput{
		ProjectID:                 req.ProjectID,
		TraceID:                   traceID,
		RunID:                     runID,
		TaskType:                  req.TaskType,
		PipelineVersion:           req.PipelineVersion,
		PolicyVersionStr:          req.PolicyVersionStr,
		InputHash:                 inputHash,
		RouterPlanEvidenceAssetID: evOut.RouterPlanEvidenceAssetID,
		OptionsEvidenceAssetID:    evOut.OptionsEvidenceAssetID,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "create_task_failed",
			"message": err.Error(),
		})
		return
	}

	attachIn := BuildAttachTaskInputsInput(req.ProjectID, taskOut.Task.ID, inputRefs)
	_, err = h.AttachTaskInputs.Handle(r.Context(), usecase.AttachMultimodalTaskInputsInput{
		ProjectID: attachIn.ProjectID,
		TaskID:    attachIn.TaskID,
		Inputs:    attachIn.Inputs,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "attach_task_inputs_failed",
			"message": err.Error(),
		})
		return
	}

	resp := BuildCreateRunResponse(taskOut.Task, routeOut.Selection, routeOut.RoutePlanCanonicalJSON, routeOut.OptionsCanonicalJSON)
	writeJSONBody(w, http.StatusCreated, resp)
}

func (h *V22RuntimeHandler) handleRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONBody(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method_not_allowed",
		})
		return
	}
	if h.GetRunDetail == nil {
		writeJSONBody(w, http.StatusInternalServerError, map[string]any{
			"error": "runtime_detail_handler_not_configured",
		})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/admin/atlaskernel/ai-runtime/runs/")
	taskID, err := strconv.ParseInt(path, 10, 64)
	if err != nil || taskID <= 0 {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "invalid_run_id",
		})
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		projectID = strings.TrimSpace(r.Header.Get("X-Project-Id"))
	}
	if projectID == "" {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "project_id_required",
		})
		return
	}

	out, err := h.GetRunDetail.Handle(context.Background(), usecase.GetRuntimeRunDetailInput{
		ProjectID: projectID,
		TaskID:    taskID,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "get_runtime_run_detail_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSONBody(w, http.StatusOK, out.Detail)
}

func defsToJSON(defs []run.EngineDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"capability":    d.Capability,
			"kind":          d.Kind,
			"display_name":  d.DisplayName,
			"provider":      d.Provider,
			"version":       d.Version,
			"enabled":       d.Enabled,
			"default_order": d.DefaultOrder,
		})
	}
	return out
}

func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}