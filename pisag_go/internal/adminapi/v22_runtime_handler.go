package adminapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
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

	RegisterUploadedEvidence   *usecase.RegisterRuntimeUploadedEvidenceUseCase
	GetUploadedEvidenceSummary *usecase.GetRuntimeUploadedEvidenceSummaryUseCase
}

func (h *V22RuntimeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/catalog", h.handleCatalog)
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/runs", h.handleRuns)
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/runs/", h.handleRunByID)

	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/evidence/upload", h.handleEvidenceUpload)
	mux.HandleFunc("/api/admin/atlaskernel/ai-runtime/evidence/", h.handleEvidenceByID)
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
		ProjectID:              req.ProjectID,
		TraceID:                traceID,
		OptionsCanonicalJSON:   CompactJSON(routeOut.OptionsCanonicalJSON),
		RoutePlanCanonicalJSON: CompactJSON(routeOut.RoutePlanCanonicalJSON),
		OptionsSHA256:          optionsSHA,
		RoutePlanSHA256:        routeSHA,
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

	out, err := h.GetRunDetail.Handle(r.Context(), usecase.GetRuntimeRunDetailInput{
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

func (h *V22RuntimeHandler) handleEvidenceUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONBody(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method_not_allowed",
		})
		return
	}
	if h.RegisterUploadedEvidence == nil {
		writeJSONBody(w, http.StatusInternalServerError, map[string]any{
			"error": "evidence_upload_handler_not_configured",
		})
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_multipart_form",
			"message": err.Error(),
		})
		return
	}

	projectID := strings.TrimSpace(r.FormValue("project_id"))
	taskType := strings.TrimSpace(r.FormValue("task_type"))
	inputRole := strings.TrimSpace(r.FormValue("input_role"))
	originalFilename := strings.TrimSpace(r.FormValue("original_filename"))

	if projectID == "" {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "project_id_required",
		})
		return
	}
	if taskType == "" {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "task_type_required",
		})
		return
	}
	if inputRole == "" {
		inputRole = "primary"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error":   "file_required",
			"message": err.Error(),
		})
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error":   "read_file_failed",
			"message": err.Error(),
		})
		return
	}
	if len(raw) == 0 {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "empty_file",
		})
		return
	}

	filename := originalFilename
	if filename == "" && header != nil {
		filename = header.Filename
	}
	filename = sanitizeFilename(filename)
	if filename == "" {
		filename = "upload.bin"
	}

	contentType := ""
	if header != nil {
		contentType = header.Header.Get("Content-Type")
	}
	contentType = normalizeContentType(contentType, filename, raw)

	if err := ValidateRuntimeEvidenceUpload(taskType, contentType); err != nil {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_upload_content_type",
			"message": err.Error(),
		})
		return
	}

	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])

	traceID := GenerateTraceIDOrDefault(strings.TrimSpace(r.FormValue("trace_id")))

	storedPath, err := saveRuntimeUploadFile(projectID, sha, filename, raw)
	if err != nil {
		writeJSONBody(w, http.StatusInternalServerError, map[string]any{
			"error":   "save_upload_file_failed",
			"message": err.Error(),
		})
		return
	}

	out, err := h.RegisterUploadedEvidence.Handle(r.Context(), run.RegisterRuntimeUploadedEvidenceInput{
		ProjectID:        projectID,
		TraceID:          traceID,
		TaskType:         taskType,
		InputRole:        inputRole,
		OriginalFilename: filename,
		ContentType:      contentType,
		SHA256:           sha,
		SizeBytes:        int64(len(raw)),
		SourceURI:        storedPath,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "upload_evidence_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSONBody(w, http.StatusCreated, map[string]any{
		"evidence_id": out.EvidenceAssetID,
		"kind":        out.Kind,
		"bytes":       out.Bytes,
		"sha256":      out.SHA256,
		"filename":    out.Filename,
		"source_uri":  storedPath,
		"input_ref":   BuildRuntimeEvidenceInputRef(out.EvidenceAssetID, inputRole, out.SHA256, out.Kind, out.Bytes),
	})
}

func (h *V22RuntimeHandler) handleEvidenceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONBody(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method_not_allowed",
		})
		return
	}
	if h.GetUploadedEvidenceSummary == nil {
		writeJSONBody(w, http.StatusInternalServerError, map[string]any{
			"error": "evidence_summary_handler_not_configured",
		})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/admin/atlaskernel/ai-runtime/evidence/")
	evidenceID, err := strconv.ParseInt(strings.TrimSpace(path), 10, 64)
	if err != nil || evidenceID <= 0 {
		writeJSONBody(w, http.StatusBadRequest, map[string]any{
			"error": "invalid_evidence_id",
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

	out, err := h.GetUploadedEvidenceSummary.Handle(r.Context(), run.GetRuntimeUploadedEvidenceSummaryInput{
		ProjectID:  projectID,
		EvidenceID: evidenceID,
	})
	if err != nil {
		writeJSONBody(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "get_evidence_summary_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSONBody(w, http.StatusOK, map[string]any{
		"evidence_id": out.EvidenceAssetID,
		"kind":        out.Kind,
		"bytes":       out.Bytes,
		"sha256":      out.SHA256,
		"filename":    out.Filename,
	})
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

func sanitizeFilename(v string) string {
	name := strings.TrimSpace(v)
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\x00", "")
	return name
}

func normalizeContentType(headerValue string, filename string, raw []byte) string {
	ct := strings.TrimSpace(headerValue)
	if ct != "" {
		if mediaType, _, err := mime.ParseMediaType(ct); err == nil && mediaType != "" {
			return mediaType
		}
		return ct
	}

	detected := http.DetectContentType(raw)
	if detected != "" && detected != "application/octet-stream" {
		return detected
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func BuildRuntimeEvidenceInputRef(evidenceID int64, inputRole, sha256Hex, kind string, sizeBytes int64) map[string]any {
	return map[string]any{
		"input_role":  inputRole,
		"evidence_id": evidenceID,
		"seq":         1,
		"sha256":      sha256Hex,
		"kind":        kind,
		"bytes":       sizeBytes,
	}
}

func ValidateRuntimeEvidenceUpload(taskType, contentType string) error {
	switch taskType {
	case "ocr":
		if allowedContentType(contentType, "application/pdf", "image/png", "image/jpeg", "image/webp", "text/plain") {
			return nil
		}
		return fmt.Errorf("content_type %q is not allowed for task_type=%s", contentType, taskType)

	case "vision":
		if allowedContentType(contentType, "application/pdf", "image/png", "image/jpeg", "image/webp") {
			return nil
		}
		return fmt.Errorf("content_type %q is not allowed for task_type=%s", contentType, taskType)

	case "fulltext_extract", "preprocess", "docparse", "embedding", "llm":
		return nil

	default:
		return fmt.Errorf("unsupported task_type=%s", taskType)
	}
}

func allowedContentType(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func saveRuntimeUploadFile(projectID, shaHex, filename string, raw []byte) (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("AK_RUNTIME_UPLOAD_DIR"))
	if baseDir == "" {
		baseDir = "./var/runtime_uploads"
	}

	safeProjectID := strings.ReplaceAll(strings.TrimSpace(projectID), "/", "_")
	if safeProjectID == "" {
		safeProjectID = "default_project"
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".bin"
	}

	dir := filepath.Join(baseDir, safeProjectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir runtime upload dir: %w", err)
	}

	finalPath := filepath.Join(dir, shaHex+ext)
	if err := os.WriteFile(finalPath, raw, 0o644); err != nil {
		return "", fmt.Errorf("write runtime upload file: %w", err)
	}

	absPath, err := filepath.Abs(finalPath)
	if err != nil {
		return "", fmt.Errorf("abs runtime upload file path: %w", err)
	}
	return absPath, nil
}