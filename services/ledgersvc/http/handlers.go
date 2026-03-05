package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ledgersvc/postgres"
)

type Handler struct {
	IngestRead   *postgres.LedgerIngestRunReadRepo
	EvidenceRead *postgres.EvidenceReadRepo
}

func NewHandler(ing *postgres.LedgerIngestRunReadRepo, ev *postgres.EvidenceReadRepo) *Handler {
	return &Handler{IngestRead: ing, EvidenceRead: ev}
}

// Very small router:
// - GET /v1/projects/{project_id}/ledger/ingest-runs
// - GET /v1/projects/{project_id}/ledger/ingest-runs/{ingest_run_id}
// - GET /v1/projects/{project_id}/evidence/{evidence_ref}
// - GET /v1/projects/{project_id}/evidence/{evidence_ref}/content  (NEW)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	traceID := GetOrCreateTraceID(r)

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	// Expect: v1/projects/{project_id}/...
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "projects" {
		WriteError(w, traceID, http.StatusNotFound, "NotFound", "route not found")
		return
	}
	projectID := parts[2]
	if strings.TrimSpace(projectID) == "" {
		WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "project_id required")
		return
	}

	// /v1/projects/{project_id}/ledger/ingest-runs...
	if len(parts) >= 5 && parts[3] == "ledger" && parts[4] == "ingest-runs" {
		if r.Method != http.MethodGet {
			WriteError(w, traceID, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET only")
			return
		}
		// list
		if len(parts) == 5 {
			h.handleListIngestRuns(w, r, traceID, projectID)
			return
		}
		// detail
		if len(parts) == 6 {
			h.handleGetIngestRun(w, r, traceID, projectID, parts[5])
			return
		}
	}

	// ✅ NEW: /v1/projects/{project_id}/evidence/{evidence_ref}/content
	if len(parts) == 6 && parts[3] == "evidence" && parts[5] == "content" {
		if r.Method != http.MethodGet {
			WriteError(w, traceID, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET only")
			return
		}
		h.handleGetEvidenceContent(w, r, traceID, projectID, parts[4])
		return
	}

	// /v1/projects/{project_id}/evidence/{evidence_ref}
	if len(parts) == 5 && parts[3] == "evidence" {
		if r.Method != http.MethodGet {
			WriteError(w, traceID, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET only")
			return
		}
		h.handleGetEvidence(w, r, traceID, projectID, parts[4])
		return
	}

	WriteError(w, traceID, http.StatusNotFound, "NotFound", "route not found")
}

func (h *Handler) handleListIngestRuns(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	q := r.URL.Query()

	var status *string
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		status = &s
	}

	var from *time.Time
	if s := strings.TrimSpace(q.Get("from")); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "from must be RFC3339")
			return
		}
		from = &t
	}

	var to *time.Time
	if s := strings.TrimSpace(q.Get("to")); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "to must be RFC3339")
			return
		}
		to = &t
	}

	limit := 50
	if s := strings.TrimSpace(q.Get("limit")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "limit must be int")
			return
		}
		limit = n
	}

	rows, err := h.IngestRead.List(r.Context(), projectID, status, from, to, limit)
	if err != nil {
		WriteError(w, traceID, http.StatusInternalServerError, "Error", err.Error())
		return
	}

	WriteJSON(w, traceID, http.StatusOK, map[string]any{
		"items": rows,
	})
}

func (h *Handler) handleGetIngestRun(w http.ResponseWriter, r *http.Request, traceID, projectID, ingestRunID string) {
	row, err := h.IngestRead.Get(r.Context(), projectID, ingestRunID)
	if err != nil {
		if err == postgres.ErrReadNotFound {
			WriteError(w, traceID, http.StatusNotFound, "NotFound", "ingest_run not found")
			return
		}
		WriteError(w, traceID, http.StatusInternalServerError, "Error", err.Error())
		return
	}
	WriteJSON(w, traceID, http.StatusOK, row)
}

func (h *Handler) handleGetEvidence(w http.ResponseWriter, r *http.Request, traceID, projectID, evidenceRef string) {
	row, err := h.EvidenceRead.GetByRef(r.Context(), projectID, evidenceRef)
	if err != nil {
		if err == postgres.ErrReadNotFound {
			WriteError(w, traceID, http.StatusNotFound, "NotFound", "evidence not found")
			return
		}
		WriteError(w, traceID, http.StatusInternalServerError, "Error", err.Error())
		return
	}
	WriteJSON(w, traceID, http.StatusOK, row)
}

// ✅ NEW: evidence content (generated JSON only, stored on filesystem)
// Reads: ${LEDGERSVC_EVIDENCE_DIR:-/var/ledgersvc/evidence}/{project_id}/{evidence_ref}.json
func (h *Handler) handleGetEvidenceContent(w http.ResponseWriter, r *http.Request, traceID, projectID, evidenceRef string) {
	// 1) metadata check (exists + project match)
	row, err := h.EvidenceRead.GetByRef(r.Context(), projectID, evidenceRef)
	if err != nil {
		if err == postgres.ErrReadNotFound {
			WriteError(w, traceID, http.StatusNotFound, "NotFound", "evidence not found")
			return
		}
		WriteError(w, traceID, http.StatusInternalServerError, "Error", err.Error())
		return
	}

	// 2) only support generated evidence for now (reject list is generated)
	if row.SourceKind != "generated" {
		WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "content endpoint supports generated evidence only")
		return
	}
	if row.MimeType != "application/json" {
		WriteError(w, traceID, http.StatusBadRequest, "InvalidArgument", "content endpoint supports application/json only")
		return
	}

	// 3) read file
	dir := strings.TrimSpace(os.Getenv("LEDGERSVC_EVIDENCE_DIR"))
	if dir == "" {
		dir = "/var/ledgersvc/evidence"
	}
	path := filepath.Join(dir, projectID, evidenceRef+".json")

	b, rerr := os.ReadFile(path)
	if rerr != nil {
		WriteError(w, traceID, http.StatusNotFound, "NotFound", "evidence content file not found")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set(TraceHeader, traceID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// optional helper: pretty print raw JSON fields if you need; currently not used.
func toRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}