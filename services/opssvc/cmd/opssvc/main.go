package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultPort = "9011"
	traceHeader = "X-Trace-Id"
	serviceName = "opssvc"
	defaultEvidenceDir = "/var/opssvc/evidence"
)

// --------------------
// Runbooks
// --------------------
type RunbookUpsertRequest struct {
	RunbookKey     string   `json:"runbook_key"`
	Title          string   `json:"title"`
	Steps          any      `json:"steps"`
	SafetyChecks   any      `json:"safety_checks"`
	RequiredRoles  []string `json:"required_roles"`
	Status         string   `json:"status"`
	CreatedByType  string   `json:"created_by_type"`
	CreatedByID    string   `json:"created_by_id"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// --------------------
// Ops Actions (remediation_*)
// --------------------
type ActionProposeRequest struct {
	IncidentID       int64  `json:"incident_id"`
	ProposalKey      string `json:"proposal_key"`
	ProposalType     string `json:"proposal_type"`
	RiskLevel        string `json:"risk_level"`
	RequiresApproval *bool  `json:"requires_approval"`

	Plan    any `json:"plan"`
	Impact  any `json:"impact"`
	Primary any `json:"primary"`

	ExpiresAtUTC *time.Time `json:"expires_at_utc"`

	CreatedByUserID string `json:"created_by_user_id"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ActionDecideRequest struct {
	Confirm        bool   `json:"confirm"`
	UserID         string `json:"user_id"`
	ReasonSummary  string `json:"reason_summary"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ActionExecuteRequest struct {
	Confirm        bool   `json:"confirm"`
	ExecutorUserID string `json:"executor_user_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// --------------------
// Incidents
// --------------------
type IncidentCreateRequest struct {
	IncidentKey    string `json:"incident_key"`
	Severity       string `json:"severity"`
	IncidentType   string `json:"incident_type"`
	DetectedBy     string `json:"detected_by"`
	Status         string `json:"status"`
	OwnerUserID    string `json:"owner_user_id"`
	RootTraceID    string `json:"root_trace_id"`
	RootRunID      string `json:"root_run_id"`
	Summary        any    `json:"summary"`
	Primary        any    `json:"primary"`
	IdempotencyKey string `json:"idempotency_key"`
}

type IncidentEventAppendRequest struct {
	EventType      string `json:"event_type"`
	CreatedByType  string `json:"created_by_type"`
	CreatedByID    string `json:"created_by_id"`
	Body           any    `json:"body"`
	IdempotencyKey string `json:"idempotency_key"`
}

type IncidentStatusUpdateRequest struct {
	Status         string `json:"status"`
	Resolved       bool   `json:"resolved"`
	UserID         string `json:"user_id"`
	Note           string `json:"note"`
	IdempotencyKey string `json:"idempotency_key"`
}

// --------------------
// main
// --------------------
func main() {
	port := os.Getenv("OPSSVC_PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "service": serviceName, "now": time.Now().Format(time.RFC3339Nano), "trace_id": traceID,
		})
	})

	mux.HandleFunc("/health/db", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		conn, err := openDB(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
			return
		}
		defer conn.Close(r.Context())
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var one int
		if err := conn.QueryRow(ctx, "select 1").Scan(&one); err != nil || one != 1 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "query_failed", "detail": errString(err), "trace_id": traceID})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"db": "ak_postgres", "ok": true, "select_1": one, "service": serviceName, "trace_id": traceID})
	})

	mux.HandleFunc("/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 3 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}
		projectID := parts[0]
		if parts[1] != "ops" {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		// ----- Incidents -----
		if parts[2] == "incidents" {
			if len(parts) == 3 {
				if r.Method == http.MethodGet {
					handleIncidentList(w, r, traceID, projectID)
					return
				}
				if r.Method == http.MethodPost {
					handleIncidentCreate(w, r, traceID, projectID)
					return
				}
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}
			if len(parts) == 4 && r.Method == http.MethodGet {
				handleIncidentGet(w, r, traceID, projectID, parts[3])
				return
			}
			if len(parts) == 5 && parts[4] == "events" && r.Method == http.MethodPost {
				handleIncidentEventAppend(w, r, traceID, projectID, parts[3])
				return
			}
			if len(parts) == 5 && parts[4] == "status" && r.Method == http.MethodPost {
				handleIncidentStatusUpdate(w, r, traceID, projectID, parts[3])
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		// ----- Runbooks -----
		if parts[2] == "runbooks" {
			if len(parts) == 3 {
				if r.Method == http.MethodGet {
					handleRunbookList(w, r, traceID, projectID)
					return
				}
				if r.Method == http.MethodPost {
					handleRunbookUpsert(w, r, traceID, projectID)
					return
				}
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}
			if len(parts) == 4 && r.Method == http.MethodGet {
				handleRunbookGet(w, r, traceID, projectID, parts[3])
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		// ----- Actions -----
		if parts[2] == "actions" && len(parts) == 4 && parts[3] == "propose" && r.Method == http.MethodPost {
			handleActionPropose(w, r, traceID, projectID)
			return
		}
		if parts[2] == "actions" && len(parts) == 3 && r.Method == http.MethodGet {
			handleActionList(w, r, traceID, projectID)
			return
		}
		if parts[2] == "actions" && len(parts) >= 4 {
			proposalID := parts[3]
			if len(parts) == 4 && r.Method == http.MethodGet {
				handleActionGet(w, r, traceID, projectID, proposalID)
				return
			}
			if len(parts) == 5 && r.Method == http.MethodPost {
				switch parts[4] {
				case "approve":
					handleActionApprove(w, r, traceID, projectID, proposalID)
					return
				case "reject":
					handleActionReject(w, r, traceID, projectID, proposalID)
					return
				case "execute":
					handleActionExecute(w, r, traceID, projectID, proposalID)
					return
				}
			}
		}

		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{"service": serviceName, "message": "opssvc up", "trace_id": traceID})
	})

	addr := "0.0.0.0:" + port
	log.Printf("[%s] listening on %s", serviceName, addr)
	if err := http.ListenAndServe(addr, withLogging(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// ============================================================
// Incidents
// ============================================================
func handleIncidentCreate(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	var req IncidentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID})
		return
	}
	if req.Summary == nil || strings.TrimSpace(req.IncidentType) == "" || strings.TrimSpace(req.Severity) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"severity, incident_type, summary required","trace_id":traceID})
		return
	}
	if req.IncidentKey == "" {
		req.IncidentKey = "inc-" + traceID
	}
	if req.Status == "" {
		req.Status = "open"
	}
	if req.DetectedBy == "" {
		req.DetectedBy = "manual"
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "inc-" + newTraceID()
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	_ = os.MkdirAll(evidenceDir, 0o755)

	sumID, sumRef, err := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc", "generated",
		writeJSONFile(evidenceDir, "incident_summary", req.Summary), req.IdempotencyKey+"-summary")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	var primaryID *int64 = nil
	var primaryRef string = ""
	if req.Primary != nil {
		id, ref, e := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc", "generated",
			writeJSONFile(evidenceDir, "incident_primary", req.Primary), req.IdempotencyKey+"-primary")
		if e != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":e.Error(),"trace_id":traceID})
			return
		}
		primaryID = &id
		primaryRef = ref
	}

	var rootTrace any = nil
	if strings.TrimSpace(req.RootTraceID) != "" { rootTrace = req.RootTraceID }
	var rootRun any = nil
	if strings.TrimSpace(req.RootRunID) != "" { rootRun = req.RootRunID }

	var incidentID int64
	err = conn.QueryRow(ctx, `
	  INSERT INTO incidents(
	    project_id, incident_key, status, severity, incident_type,
	    root_trace_id, root_run_id,
	    detected_by, incident_summary_evidence_asset_id, primary_evidence_asset_id,
	    owner_user_id
	  )
	  VALUES($1,$2,$3,$4,$5,$6::uuid,$7::uuid,$8,$9,$10,$11)
	  ON CONFLICT (project_id, incident_key)
	  DO UPDATE SET updated_at=now()
	  RETURNING id
	`, projectID, req.IncidentKey, req.Status, strings.ToUpper(req.Severity), req.IncidentType,
		rootTrace, rootRun, req.DetectedBy, sumID, primaryID, nullIfEmptyString(req.OwnerUserID)).Scan(&incidentID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"incident_id": incidentID,
		"incident_key": req.IncidentKey,
		"incident_summary_evidence_asset_id": sumID,
		"incident_summary_evidence_ref": sumRef,
		"primary_evidence_asset_id": primaryID,
		"primary_evidence_ref": primaryRef,
		"trace_id": traceID,
	})
}

func handleIncidentList(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	query := `
	  SELECT id, project_id, incident_key, status, severity, incident_type,
	         detected_by, detected_at_utc, resolved_at_utc,
	         incident_summary_evidence_asset_id, primary_evidence_asset_id,
	         owner_user_id, created_at, updated_at
	    FROM incidents
	   WHERE project_id=$1
	`
	args := []any{projectID}
	idx := 2
	if status != "" {
		query += " AND lower(status)=lower($" + strconv.Itoa(idx) + ")"
		args = append(args, status)
		idx++
	}
	if severity != "" {
		query += " AND upper(severity)=upper($" + strconv.Itoa(idx) + ")"
		args = append(args, severity)
		idx++
	}
	query += " ORDER BY detected_at_utc DESC LIMIT $" + strconv.Itoa(idx) + " OFFSET $" + strconv.Itoa(idx+1)
	args = append(args, limit, offset)

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var (
			id int64
			proj, ikey, st, sev, itype, dby string
			dat time.Time
			res *time.Time
			sumID int64
			priID *int64
			owner *string
			created, updated time.Time
		)
		if err := rows.Scan(&id,&proj,&ikey,&st,&sev,&itype,&dby,&dat,&res,&sumID,&priID,&owner,&created,&updated); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"scan_failed","detail":err.Error(),"trace_id":traceID})
			return
		}
		out = append(out, map[string]any{
			"id": id, "project_id": proj, "incident_key": ikey, "status": st, "severity": sev, "incident_type": itype,
			"detected_by": dby, "detected_at_utc": dat, "resolved_at_utc": res,
			"incident_summary_evidence_asset_id": sumID, "primary_evidence_asset_id": priID, "owner_user_id": owner,
			"created_at": created, "updated_at": updated,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": out, "trace_id": traceID})
}

func handleIncidentGet(w http.ResponseWriter, r *http.Request, traceID, projectID, incidentID string) {
	id, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_incident_id","trace_id":traceID})
		return
	}
	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var (
		proj, ikey, st, sev, itype, dby string
		rootTrace *string
		rootRun *string
		dat time.Time
		sumID int64
		priID *int64
		owner *string
		res *time.Time
		created, updated time.Time
	)
	err = conn.QueryRow(ctx, `
	  SELECT project_id, incident_key, status, severity, incident_type,
	         root_trace_id::text, root_run_id::text,
	         detected_by, detected_at_utc,
	         incident_summary_evidence_asset_id, primary_evidence_asset_id,
	         owner_user_id, resolved_at_utc, created_at, updated_at
	    FROM incidents
	   WHERE id=$1 AND project_id=$2
	`, id, projectID).Scan(&proj,&ikey,&st,&sev,&itype,&rootTrace,&rootRun,&dby,&dat,&sumID,&priID,&owner,&res,&created,&updated)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": map[string]any{
		"id": id, "project_id": proj, "incident_key": ikey, "status": st, "severity": sev, "incident_type": itype,
		"root_trace_id": rootTrace, "root_run_id": rootRun,
		"detected_by": dby, "detected_at_utc": dat,
		"incident_summary_evidence_asset_id": sumID, "primary_evidence_asset_id": priID,
		"owner_user_id": owner, "resolved_at_utc": res,
		"created_at": created, "updated_at": updated,
	}, "trace_id": traceID})
}

func handleIncidentEventAppend(w http.ResponseWriter, r *http.Request, traceID, projectID, incidentID string) {
	id, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_incident_id","trace_id":traceID})
		return
	}
	var req IncidentEventAppendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID})
		return
	}
	if strings.TrimSpace(req.EventType) == "" || req.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"event_type and body required","trace_id":traceID})
		return
	}
	if req.CreatedByType == "" { req.CreatedByType = "system" }
	if req.IdempotencyKey == "" { req.IdempotencyKey = "incev-" + newTraceID() }

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	_ = os.MkdirAll(evidenceDir, 0o755)

	evID, evRef, err := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, req.CreatedByType, req.CreatedByID, "generated",
		writeJSONFile(evidenceDir, "incident_event", map[string]any{
			"incident_id": id, "event_type": req.EventType, "body": req.Body,
			"created_by_type": req.CreatedByType, "created_by_id": req.CreatedByID,
			"at": time.Now().Format(time.RFC3339Nano),
		}), req.IdempotencyKey)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	var eventRowID int64
	err = conn.QueryRow(ctx, `
	  INSERT INTO incident_events(project_id, incident_id, event_type, event_evidence_asset_id, created_by_type, created_by_id)
	  VALUES($1,$2,$3,$4,$5,$6)
	  RETURNING id
	`, projectID, id, req.EventType, evID, req.CreatedByType, nullIfEmptyString(req.CreatedByID)).Scan(&eventRowID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"incident_event_id": eventRowID,
		"event_evidence_asset_id": evID,
		"event_evidence_ref": evRef,
		"trace_id": traceID,
	})
}

func handleIncidentStatusUpdate(w http.ResponseWriter, r *http.Request, traceID, projectID, incidentID string) {
	id, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_incident_id","trace_id":traceID})
		return
	}
	var req IncidentStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID})
		return
	}
	if strings.TrimSpace(req.Status) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_status","trace_id":traceID})
		return
	}
	if req.IdempotencyKey == "" { req.IdempotencyKey = "incst-" + newTraceID() }

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var resolvedAt any = nil
	if req.Resolved || strings.ToLower(req.Status) == "resolved" || strings.ToLower(req.Status) == "closed" {
		resolvedAt = time.Now().UTC()
	}

	_, err = conn.Exec(ctx, `
	  UPDATE incidents
	     SET status=$1,
	         resolved_at_utc=COALESCE($2,resolved_at_utc),
	         updated_at=now()
	   WHERE id=$3 AND project_id=$4
	`, req.Status, resolvedAt, id, projectID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	// best-effort note event
	if strings.TrimSpace(req.Note) != "" {
		_ = appendIncidentEventBestEffort(ctx, conn, projectID, id, "note", map[string]any{"status":req.Status,"note":req.Note,"user_id":req.UserID}, traceID, req.IdempotencyKey+"-note")
	}

	writeJSON(w, http.StatusOK, map[string]any{"incident_id": id, "status": req.Status, "trace_id": traceID})
}

func appendIncidentEventBestEffort(ctx context.Context, conn *pgx.Conn, projectID string, incidentID int64, eventType string, body any, traceID string, idem string) error {
	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	_ = os.MkdirAll(evidenceDir, 0o755)

	evID, _, err := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc", "generated",
		writeJSONFile(evidenceDir, "incident_event", map[string]any{"incident_id":incidentID,"event_type":eventType,"body":body}), idem)
	if err != nil { return err }

	_, err = conn.Exec(ctx, `
	  INSERT INTO incident_events(project_id, incident_id, event_type, event_evidence_asset_id, created_by_type, created_by_id)
	  VALUES($1,$2,$3,$4,'system','opssvc')
	`, projectID, incidentID, eventType, evID)
	return err
}

// ============================================================
// Runbooks (unchanged)
// ============================================================
func handleRunbookUpsert(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	var req RunbookUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
		return
	}
	if req.RunbookKey == "" || req.Title == "" || req.Steps == nil || req.SafetyChecks == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"runbook_key,title,steps,safety_checks required","trace_id":traceID})
		return
	}
	if req.Status == "" { req.Status = "active" }
	if req.CreatedByType == "" { req.CreatedByType = "system" }
	if req.CreatedByID == "" { req.CreatedByID = "opssvc" }
	if req.IdempotencyKey == "" { req.IdempotencyKey = "rbk-" + newTraceID() }

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_dir_unavailable","detail":err.Error(),"trace_id":traceID})
		return
	}

	stepsBytes, _ := json.Marshal(req.Steps)
	stepsSha := sha256Hex(stepsBytes)
	stepsPath := filepath.Join(evidenceDir, "runbook_steps_"+stepsSha+".json")
	if err := writeFileIfNotExists(stepsPath, stepsBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_write_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	stepsRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "opssvc", "text", "application/json", "generated", stepsPath, stepsSha, int64(len(stepsBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-steps")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	checkBytes, _ := json.Marshal(req.SafetyChecks)
	checkSha := sha256Hex(checkBytes)
	checkPath := filepath.Join(evidenceDir, "runbook_safety_"+checkSha+".json")
	if err := writeFileIfNotExists(checkPath, checkBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_write_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	checkRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "opssvc", "text", "application/json", "generated", checkPath, checkSha, int64(len(checkBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-checks")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID})
		return
	}

	roles := req.RequiredRoles
	if roles == nil { roles = []string{} }

	var runbookID *string
	err = conn.QueryRow(ctx, `
		SELECT ops_v11.runbook_upsert_v11(
			$1::varchar,$2::varchar,$3::varchar,$4::uuid,$5::uuid,$6::text[],$7::varchar,$8::varchar,$9::varchar
		)::text
	`, projectID, req.RunbookKey, req.Title, stepsRef, checkRef, roles, req.Status, req.CreatedByType, req.CreatedByID).Scan(&runbookID)
	if err != nil || runbookID == nil || *runbookID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":errString(err),"trace_id":traceID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbook_id": *runbookID, "steps_evidence_ref": stepsRef, "safety_checks_evidence_ref": checkRef, "trace_id": traceID})
}

func handleRunbookList(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	status := r.URL.Query().Get("status")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	var statusArg any = nil
	if status != "" { statusArg = status }

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := conn.Query(ctx, `
		SELECT id::text, project_id, runbook_key, title,
		       steps_evidence_ref::text, safety_checks_evidence_ref::text,
		       required_roles, status,
		       created_by_type, COALESCE(created_by_id,''),
		       created_at, updated_at
		  FROM ops_v11.runbook_list_v11($1::varchar,$2::varchar,$3::int,$4::int)
	`, projectID, statusArg, limit, offset)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, proj, key, title, stepsRef, checksRef, st, cbt, cbid string
		var roles []string
		var created, updated time.Time
		if err := rows.Scan(&id,&proj,&key,&title,&stepsRef,&checksRef,&roles,&st,&cbt,&cbid,&created,&updated); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"scan_failed","detail":err.Error(),"trace_id":traceID}); return
		}
		out = append(out, map[string]any{
			"id": id, "project_id": proj, "runbook_key": key, "title": title,
			"steps_evidence_ref": stepsRef, "safety_checks_evidence_ref": checksRef,
			"required_roles": roles, "status": st, "created_by_type": cbt, "created_by_id": cbid,
			"created_at": created, "updated_at": updated,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbooks": out, "trace_id": traceID})
}

func handleRunbookGet(w http.ResponseWriter, r *http.Request, traceID, projectID, runbookID string) {
	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := conn.QueryRow(ctx, `
		SELECT id::text, project_id, runbook_key, title,
		       steps_evidence_ref::text, safety_checks_evidence_ref::text,
		       required_roles, status,
		       created_by_type, COALESCE(created_by_id,''),
		       created_at, updated_at
		  FROM ops_v11.runbook_get_v11($1::varchar,$2::uuid)
	`, projectID, runbookID)

	var id, proj, key, title, stepsRef, checksRef, st, cbt, cbid string
	var roles []string
	var created, updated time.Time
	if err := row.Scan(&id,&proj,&key,&title,&stepsRef,&checksRef,&roles,&st,&cbt,&cbid,&created,&updated); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbook": map[string]any{
		"id": id, "project_id": proj, "runbook_key": key, "title": title,
		"steps_evidence_ref": stepsRef, "safety_checks_evidence_ref": checksRef,
		"required_roles": roles, "status": st, "created_by_type": cbt, "created_by_id": cbid,
		"created_at": created, "updated_at": updated,
	}, "trace_id": traceID})
}

// ============================================================
// Actions (minimal: propose/list/get/approve/reject/execute)
// ============================================================
func handleActionPropose(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	var req ActionProposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID}); return
	}
	if req.IncidentID == 0 || strings.TrimSpace(req.ProposalType) == "" || strings.TrimSpace(req.RiskLevel) == "" || req.Plan == nil || req.Impact == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"incident_id, proposal_type, risk_level, plan, impact required","trace_id":traceID}); return
	}
	requiresApproval := true
	if req.RequiresApproval != nil { requiresApproval = *req.RequiresApproval }
	if req.ProposalKey == "" { req.ProposalKey = "ops-" + traceID }
	if req.IdempotencyKey == "" { req.IdempotencyKey = "opsprop-" + newTraceID() }

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_dir_unavailable","detail":err.Error(),"trace_id":traceID}); return
	}

	planID, planRef, err := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc",
		"generated", writeJSONFile(evidenceDir, "action_plan", req.Plan), req.IdempotencyKey+"-plan")
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID}); return }

	impactID, impactRef, err := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc",
		"generated", writeJSONFile(evidenceDir, "action_impact", req.Impact), req.IdempotencyKey+"-impact")
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":err.Error(),"trace_id":traceID}); return }

	var primaryID *int64 = nil
	var primaryRef string = ""
	if req.Primary != nil {
		id, ref, e := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc",
			"generated", writeJSONFile(evidenceDir, "action_primary", req.Primary), req.IdempotencyKey+"-primary")
		if e != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"evidence_register_failed","detail":e.Error(),"trace_id":traceID}); return }
		primaryID = &id
		primaryRef = ref
	}

	status := "proposed"
	if requiresApproval { status = "needs_review" }

	var proposalRowID int64
	err = conn.QueryRow(ctx, `
		INSERT INTO remediation_proposals(
			project_id, incident_id, proposal_key, proposal_type, status, risk_level, requires_approval,
			proposal_plan_evidence_asset_id, proposal_impact_evidence_asset_id, proposal_primary_evidence_asset_id, expires_at_utc
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (project_id, proposal_key) DO UPDATE SET updated_at=now()
		RETURNING id
	`, projectID, req.IncidentID, req.ProposalKey, req.ProposalType, status, req.RiskLevel, requiresApproval,
		planID, impactID, primaryID, req.ExpiresAtUTC).Scan(&proposalRowID)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }

	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id": proposalRowID, "proposal_key": req.ProposalKey, "status": status,
		"plan_evidence_asset_id": planID, "plan_evidence_ref": planRef,
		"impact_evidence_asset_id": impactID, "impact_evidence_ref": impactRef,
		"primary_evidence_asset_id": primaryID, "primary_evidence_ref": primaryRef,
		"trace_id": traceID,
	})
}

func handleActionList(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	status := r.URL.Query().Get("status")
	incidentIDStr := r.URL.Query().Get("incident_id")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

	var incidentID any = nil
	if incidentIDStr != "" {
		if v, err := strconv.ParseInt(incidentIDStr, 10, 64); err == nil {
			incidentID = v
		}
	}

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	query := `
	  SELECT id, project_id, incident_id, proposal_key, proposal_type, status, risk_level, requires_approval,
	         proposal_plan_evidence_asset_id, proposal_impact_evidence_asset_id, proposal_primary_evidence_asset_id,
	         approved_by_user_id, approved_at_utc, applied_by_user_id, applied_at_utc, expires_at_utc, created_at, updated_at
	    FROM remediation_proposals
	   WHERE project_id=$1
	`
	args := []any{projectID}
	idx := 2
	if status != "" {
		query += " AND lower(status)=lower($" + strconv.Itoa(idx) + ")"
		args = append(args, status)
		idx++
	}
	if incidentID != nil {
		query += " AND incident_id=$" + strconv.Itoa(idx)
		args = append(args, incidentID)
		idx++
	}
	query += " ORDER BY updated_at DESC LIMIT $" + strconv.Itoa(idx) + " OFFSET $" + strconv.Itoa(idx+1)
	args = append(args, limit, offset)

	rows, err := conn.Query(ctx, query, args...)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var (
			id int64
			proj string
			inc int64
			pkey, ptype, pstatus, risk string
			reqAppr bool
			planID, impactID int64
			primaryID *int64
			approvedBy *string
			approvedAt *time.Time
			appliedBy *string
			appliedAt *time.Time
			expiresAt *time.Time
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&id,&proj,&inc,&pkey,&ptype,&pstatus,&risk,&reqAppr,&planID,&impactID,&primaryID,&approvedBy,&approvedAt,&appliedBy,&appliedAt,&expiresAt,&createdAt,&updatedAt); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"scan_failed","detail":err.Error(),"trace_id":traceID}); return
		}
		out = append(out, map[string]any{
			"id": id, "project_id": proj, "incident_id": inc, "proposal_key": pkey, "proposal_type": ptype,
			"status": pstatus, "risk_level": risk, "requires_approval": reqAppr,
			"proposal_plan_evidence_asset_id": planID, "proposal_impact_evidence_asset_id": impactID, "proposal_primary_evidence_asset_id": primaryID,
			"approved_by_user_id": approvedBy, "approved_at_utc": approvedAt,
			"applied_by_user_id": appliedBy, "applied_at_utc": appliedAt,
			"expires_at_utc": expiresAt, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out, "trace_id": traceID})
}

func handleActionGet(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	id, err := strconv.ParseInt(proposalID, 10, 64)
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_proposal_id","trace_id":traceID}); return }
	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var (
		proj string
		inc int64
		pkey, ptype, pstatus, risk string
		reqAppr bool
		planID, impactID int64
		primaryID *int64
		approvedBy *string
		approvedAt *time.Time
		appliedBy *string
		appliedAt *time.Time
		expiresAt *time.Time
		createdAt, updatedAt time.Time
	)
	err = conn.QueryRow(ctx, `
	  SELECT project_id, incident_id, proposal_key, proposal_type, status, risk_level, requires_approval,
	         proposal_plan_evidence_asset_id, proposal_impact_evidence_asset_id, proposal_primary_evidence_asset_id,
	         approved_by_user_id, approved_at_utc, applied_by_user_id, applied_at_utc, expires_at_utc, created_at, updated_at
	    FROM remediation_proposals
	   WHERE id=$1 AND project_id=$2
	`, id, projectID).Scan(&proj,&inc,&pkey,&ptype,&pstatus,&risk,&reqAppr,&planID,&impactID,&primaryID,&approvedBy,&approvedAt,&appliedBy,&appliedAt,&expiresAt,&createdAt,&updatedAt)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }

	writeJSON(w, http.StatusOK, map[string]any{"action": map[string]any{
		"id": id, "project_id": proj, "incident_id": inc, "proposal_key": pkey, "proposal_type": ptype, "status": pstatus,
		"risk_level": risk, "requires_approval": reqAppr,
		"proposal_plan_evidence_asset_id": planID, "proposal_impact_evidence_asset_id": impactID, "proposal_primary_evidence_asset_id": primaryID,
		"approved_by_user_id": approvedBy, "approved_at_utc": approvedAt,
		"applied_by_user_id": appliedBy, "applied_at_utc": appliedAt,
		"expires_at_utc": expiresAt, "created_at": createdAt, "updated_at": updatedAt,
	}, "trace_id": traceID})
}

func handleActionApprove(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	var req ActionDecideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID}); return
	}
	if !req.Confirm || req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"confirm and user_id required","trace_id":traceID}); return
	}
	id, err := strconv.ParseInt(proposalID, 10, 64)
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_proposal_id","trace_id":traceID}); return }

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	_, err = conn.Exec(ctx, `
	  UPDATE remediation_proposals
	     SET status='approved', approved_by_user_id=$1, approved_at_utc=now(), updated_at=now()
	   WHERE id=$2 AND project_id=$3
	`, req.UserID, id, projectID)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }

	writeJSON(w, http.StatusOK, map[string]any{"proposal_id": id, "status":"approved", "trace_id": traceID})
}

func handleActionReject(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	var req ActionDecideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID}); return
	}
	if !req.Confirm || req.UserID == "" || strings.TrimSpace(req.ReasonSummary) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"confirm, user_id, reason_summary required","trace_id":traceID}); return
	}
	id, err := strconv.ParseInt(proposalID, 10, 64)
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_proposal_id","trace_id":traceID}); return }

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	_ = os.MkdirAll(evidenceDir, 0o755)

	rej := map[string]any{"proposal_id": id, "reason": req.ReasonSummary, "rejected_by": req.UserID, "at": time.Now().Format(time.RFC3339Nano)}
	rejID, _, _ := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc", "generated",
		writeJSONFile(evidenceDir, "proposal_reject", rej), "reject-"+newTraceID())

	_, err = conn.Exec(ctx, `
	  UPDATE remediation_proposals
	     SET status='rejected', updated_at=now(),
	         proposal_primary_evidence_asset_id=COALESCE(proposal_primary_evidence_asset_id,$1)
	   WHERE id=$2 AND project_id=$3
	`, nullableBigint(rejID), id, projectID)
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return }

	writeJSON(w, http.StatusOK, map[string]any{"proposal_id": id, "status":"rejected", "trace_id": traceID})
}

func handleActionExecute(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	var req ActionExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_json","trace_id":traceID}); return
	}
	if !req.Confirm || req.ExecutorUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error":"missing_fields","detail":"confirm and executor_user_id required","trace_id":traceID}); return
	}
	if req.IdempotencyKey == "" { req.IdempotencyKey = "exec-" + newTraceID() }

	id, err := strconv.ParseInt(proposalID, 10, 64)
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]any{"error":"invalid_proposal_id","trace_id":traceID}); return }

	conn, err := openDB(r.Context())
	if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"connect_failed","detail":err.Error(),"trace_id":traceID}); return }
	defer conn.Close(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var pty, st string
	var planAssetID int64
	if err := conn.QueryRow(ctx, `SELECT proposal_type, status, proposal_plan_evidence_asset_id FROM remediation_proposals WHERE id=$1 AND project_id=$2`, id, projectID).Scan(&pty,&st,&planAssetID); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID}); return
	}
	if strings.ToLower(st) != "approved" {
		writeJSON(w, http.StatusConflict, map[string]any{"error":"not_approved","current_status":st,"trace_id":traceID}); return
	}

	var planSourceURI string
	if err := conn.QueryRow(ctx, `SELECT source_uri FROM evidence_assets WHERE id=$1 AND project_id=$2`, planAssetID, projectID).Scan(&planSourceURI); err != nil || strings.TrimSpace(planSourceURI) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status":"review_required","error":"plan_evidence_missing","trace_id":traceID}); return
	}
	planBytes, err := os.ReadFile(planSourceURI)
	if err != nil { writeJSON(w, http.StatusOK, map[string]any{"status":"review_required","error":"plan_read_failed","detail":err.Error(),"trace_id":traceID}); return }
	var plan map[string]any
	_ = json.Unmarshal(planBytes, &plan)

	runID := newUUID()
	if err := tryInsertRunMinimal(ctx, conn, runID, projectID, traceID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status":"review_required","error":"runs_insert_failed","detail":err.Error(),"trace_id":traceID}); return
	}

	evidenceDir := os.Getenv("OPSSVC_EVIDENCE_DIR")
	if evidenceDir == "" { evidenceDir = defaultEvidenceDir }
	_ = os.MkdirAll(evidenceDir, 0o755)

	actionKey := "act-" + req.IdempotencyKey

	result := map[string]any{"proposal_id": id, "proposal_type": pty, "plan": plan}
	execErr := ""
	switch strings.ToLower(pty) {
	case "policy_rollback":
		execErr = callGovsvcRollback(ctx, traceID, projectID, plan, &result)
	case "policy_retire":
		execErr = callGovsvcRetire(ctx, traceID, projectID, plan, &result)
	case "policy_publish":
		execErr = callAgentsvcPublish(ctx, traceID, projectID, plan, &result)
	default:
		execErr = "unsupported_proposal_type"
	}

	statusOut := "succeeded"
	if execErr != "" {
		statusOut = "failed"
		result["error"] = execErr
	}

	actionEvidenceID, _, eerr := evidenceRegisterV18AsAssetID(ctx, conn, projectID, traceID, "system", "opssvc",
		"generated", writeJSONFile(evidenceDir, "action_result", result), "exec-"+req.IdempotencyKey)
	if eerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status":"review_required","error":"action_evidence_register_failed","detail":eerr.Error(),"trace_id":traceID}); return
	}

	var actionRowID int64
	if err := conn.QueryRow(ctx, `
	  INSERT INTO remediation_actions(project_id, proposal_id, action_key, run_id, status, action_evidence_asset_id)
	  VALUES($1,$2,$3,$4,$5,$6)
	  ON CONFLICT (project_id, action_key) DO UPDATE SET updated_at_utc=now()
	  RETURNING id
	`, projectID, id, actionKey, runID, statusOut, actionEvidenceID).Scan(&actionRowID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status":"review_required","error":"action_insert_failed","detail":err.Error(),"trace_id":traceID}); return
	}

	if statusOut == "succeeded" {
		_, _ = conn.Exec(ctx, `UPDATE remediation_proposals SET status='applied', applied_by_user_id=$1, applied_at_utc=now(), updated_at=now() WHERE id=$2 AND project_id=$3`, req.ExecutorUserID, id, projectID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"execution_id": actionRowID,
		"proposal_id": id,
		"status": statusOut,
		"run_id": runID,
		"action_evidence_asset_id": actionEvidenceID,
		"trace_id": traceID,
	})
}

// ============================================================
// Downstream calls (govsvc/agentsvc)
// ============================================================
func callGovsvcRollback(ctx context.Context, traceID, projectID string, plan map[string]any, result *map[string]any) string {
	base := os.Getenv("GOVSVC_BASE_URL")
	if base == "" { base = "http://govsvc:9012" }
	policySetID, _ := plan["policy_set_id"].(string)
	toVersionID, _ := plan["to_version_id"].(string)
	if policySetID == "" || toVersionID == "" { return "plan_missing_policy_set_id_or_to_version_id" }
	url := base + "/v1/policies/sets/" + policySetID + "/rollback"
	body := map[string]any{"project_id": projectID, "to_version_id": toVersionID, "confirm": true, "reason": "v11 ops action rollback", "idempotency_key": "rb-" + traceID}
	return callJSON(ctx, url, traceID, body, result)
}
func callGovsvcRetire(ctx context.Context, traceID, projectID string, plan map[string]any, result *map[string]any) string {
	base := os.Getenv("GOVSVC_BASE_URL")
	if base == "" { base = "http://govsvc:9012" }
	policySetID, _ := plan["policy_set_id"].(string)
	versionID, _ := plan["version_id"].(string)
	if policySetID == "" || versionID == "" { return "plan_missing_policy_set_id_or_version_id" }
	url := base + "/v1/policies/sets/" + policySetID + "/retire"
	body := map[string]any{"project_id": projectID, "version_id": versionID, "confirm": true, "reason": "v11 ops action retire", "idempotency_key": "rt-" + traceID}
	return callJSON(ctx, url, traceID, body, result)
}
func callAgentsvcPublish(ctx context.Context, traceID, projectID string, plan map[string]any, result *map[string]any) string {
	base := os.Getenv("AGENTSVC_BASE_URL")
	if base == "" { base = "http://agentsvc:9010" }
	rpid, _ := plan["routing_proposal_id"].(string)
	if rpid == "" { return "plan_missing_routing_proposal_id" }
	url := base + "/v1/projects/" + projectID + "/routing/proposals/" + rpid + "/publish"
	body := map[string]any{"confirm": true, "published_by": "admin", "publish_reason": "v11 ops action publish", "idempotency_key": "publink-" + traceID}
	return callJSON(ctx, url, traceID, body, result)
}
func callJSON(ctx context.Context, url, traceID string, payload map[string]any, result *map[string]any) string {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(traceHeader, traceID)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil { (*result)["http_error"] = err.Error(); return "http_request_failed" }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	(*result)["http_status"] = resp.StatusCode
	(*result)["http_body"] = string(body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return "http_non2xx" }
	return ""
}

// ============================================================
// Evidence helpers (uuid ref -> bigint id)
// ============================================================
func writeJSONFile(dir, prefix string, v any) string {
	b, _ := json.Marshal(v)
	sha := sha256Hex(b)
	path := filepath.Join(dir, prefix+"_"+sha+".json")
	_ = writeFileIfNotExists(path, b)
	return path
}
func evidenceRegisterV18AsAssetID(ctx context.Context, conn *pgx.Conn, projectID, traceID, createdByType, createdByID, sourceKind, sourceURI, idemSuffix string) (int64, string, error) {
	contentBytes, err := os.ReadFile(sourceURI)
	if err != nil { return 0, "", err }
	contentSha := sha256Hex(contentBytes)

	evidenceRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, createdByType, createdByID,
		"text", "application/json", sourceKind, sourceURI, contentSha, int64(len(contentBytes)), "ja", "standard", nil, "evi-"+idemSuffix)
	if err != nil { return 0, "", err }

	var assetID int64
	if err := conn.QueryRow(ctx, `SELECT id FROM evidence_assets WHERE project_id=$1 AND evidence_ref=$2`, projectID, evidenceRef).Scan(&assetID); err != nil {
		return 0, evidenceRef, err
	}
	return assetID, evidenceRef, nil
}
func evidenceRegisterV18(ctx context.Context, conn *pgx.Conn, projectID, traceID, createdByType, createdByID, mediaType, mimeType, sourceKind, sourceURI, contentSha256 string, contentLen int64, language, retentionPolicy string, expiresAtUTC any, idempotencyKey string) (string, bool, error) {
	var evidenceRef string
	var found bool
	err := conn.QueryRow(ctx, `
		SELECT evidence_ref::text, found_existing
		  FROM public.evidence_register_v18(
		    $1::varchar, $2::varchar,
		    $3::varchar, $4::varchar,
		    $5::varchar, $6::varchar,
		    $7::varchar, $8::text,
		    $9::text, $10::bigint,
		    $11::varchar, $12::varchar,
		    $13::timestamptz,
		    $14::text
		  )
	`, projectID, traceID, createdByType, createdByID, mediaType, mimeType, sourceKind, sourceURI, contentSha256, contentLen, language, retentionPolicy, expiresAtUTC, idempotencyKey).Scan(&evidenceRef, &found)
	if err != nil { return "", false, err }
	return evidenceRef, found, nil
}

// ============================================================
// Runs minimal insert
// ============================================================
func newUUID() string {
	b := make([]byte, 16); _, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
func tryInsertRunMinimal(ctx context.Context, conn *pgx.Conn, runID, projectID, traceID string) error {
	_, err := conn.Exec(ctx, `
	  INSERT INTO runs(run_id, project_id, trace_id, status, created_at, updated_at)
	  VALUES($1::uuid,$2,$3,'created',now(),now())
	  ON CONFLICT (run_id) DO NOTHING
	`, runID, projectID, traceID)
	return err
}

// ============================================================
// Common helpers
// ============================================================
func openDB(parent context.Context) (*pgx.Conn, error) {
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" { return nil, errors.New("AK_DB_DSN is empty") }
	ctx, cancel := context.WithTimeout(parent, 2*time.Second); defer cancel()
	return pgx.Connect(ctx, dsn)
}
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
func ensureTraceID(w http.ResponseWriter, r *http.Request) string {
	v := r.Header.Get(traceHeader)
	if v == "" { v = newTraceID() }
	w.Header().Set(traceHeader, v)
	return v
}
func newTraceID() string {
	b := make([]byte, 16); _, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func writeFileIfNotExists(path string, b []byte) error {
	if _, err := os.Stat(path); err == nil { return nil }
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil { if os.IsExist(err) { return nil }; return err }
	defer f.Close()
	_, err = f.Write(b)
	return err
}
func parseIntDefault(s string, def int) int {
	if s == "" { return def }
	n, err := strconv.Atoi(s)
	if err != nil { return def }
	return n
}
func errString(err error) string {
	if err == nil { return "" }
	return err.Error()
}
func nullableBigint(v int64) any {
	if v == 0 { return nil }
	return v
}
func nullIfEmptyString(s string) any {
	if strings.TrimSpace(s) == "" { return nil }
	return s
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}