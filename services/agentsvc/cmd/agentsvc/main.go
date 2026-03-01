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
	defaultPort = "9010"
	traceHeader = "X-Trace-Id"
	serviceName = "agentsvc"

	defaultEvidenceDir = "/var/agentsvc/evidence"
)

type CreateProposalRequest struct {
	PolicySetID       string         `json:"policy_set_id"`        // uuid (gov_policy.policy_sets.id)
	PolicyVersionBase string         `json:"policy_version_base"`   // uuid or ""
	ProposalType      string         `json:"proposal_type"`         // threshold_change etc
	RiskLevel         string         `json:"risk_level"`            // low/medium/high
	RationaleSummary  string         `json:"rationale_summary"`      // <=512
	ChangeSet         map[string]any `json:"change_set"`             // evidence
	Rationale         map[string]any `json:"rationale"`              // evidence
	ImpactSummary     map[string]any `json:"impact_summary"`         // small jsonb
	IdempotencyKey    string         `json:"idempotency_key"`        // optional
}

type EvaluateProposalRequest struct {
	Confirm        bool   `json:"confirm"`
	IdempotencyKey string `json:"idempotency_key"`
}

type DecideProposalRequest struct {
	Confirm        bool   `json:"confirm"`
	ReasonSummary  string `json:"reason_summary"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PublishProposalRequest struct {
	Confirm        bool   `json:"confirm"`
	PublishedBy    string `json:"published_by"`
	PublishReason  string `json:"publish_reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

// govsvc publish response (subset)
type GovPublishResponse struct {
	PolicyVersionID           string `json:"policy_version_id"`
	PublicationID             string `json:"publication_id"`
	CompiledPolicyEvidenceRef string `json:"compiled_policy_evidence_ref"`
	CompiledPolicyChecksum    string `json:"compiled_policy_checksum"`
	TraceID                   string `json:"trace_id"`
	Status                    string `json:"status"`
}

func main() {
	port := os.Getenv("AGENTSVC_PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()

	// health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"service":  serviceName,
			"now":      time.Now().Format(time.RFC3339Nano),
			"trace_id": traceID,
		})
	})

	// health/db
	mux.HandleFunc("/health/db", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		conn, err := openDB(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ok": false, "service": serviceName, "trace_id": traceID,
				"error": "connect_failed", "detail": err.Error(),
			})
			return
		}
		defer conn.Close(r.Context())

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var one int
		if err := conn.QueryRow(ctx, "select 1").Scan(&one); err != nil || one != 1 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ok": false, "service": serviceName, "trace_id": traceID,
				"error": "query_failed", "detail": errString(err),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"db": "ak_postgres",
			"ok": true,
			"select_1": one,
			"service": serviceName,
			"trace_id": traceID,
		})
	})

	// /v1/projects/{project_id}/routing/...
	mux.HandleFunc("/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 3 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}
		projectID := parts[0]
		if parts[1] != "routing" {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		// GET /routing/evaluations/{evaluation_id}
		if len(parts) == 4 && parts[2] == "evaluations" && r.Method == http.MethodGet {
			evalID := parts[3]
			handleGetEvaluation(w, r, traceID, projectID, evalID)
			return
		}

		// proposals
		if parts[2] != "proposals" {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		// /proposals (list/create)
		if len(parts) == 3 {
			if r.Method == http.MethodGet {
				handleListProposals(w, r, traceID, projectID)
				return
			}
			if r.Method == http.MethodPost {
				handleCreateProposal(w, r, traceID, projectID)
				return
			}
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
			return
		}

		// /proposals/{proposal_id}
		if len(parts) == 4 && r.Method == http.MethodGet {
			proposalID := parts[3]
			handleGetProposal(w, r, traceID, projectID, proposalID)
			return
		}

		// /proposals/{proposal_id}/evaluate
		if len(parts) == 5 && parts[4] == "evaluate" && r.Method == http.MethodPost {
			proposalID := parts[3]
			handleEvaluateProposal(w, r, traceID, projectID, proposalID)
			return
		}

		// /proposals/{proposal_id}/approve
		if len(parts) == 5 && parts[4] == "approve" && r.Method == http.MethodPost {
			proposalID := parts[3]
			handleDecideProposal(w, r, traceID, projectID, proposalID, "approved")
			return
		}

		// /proposals/{proposal_id}/reject
		if len(parts) == 5 && parts[4] == "reject" && r.Method == http.MethodPost {
			proposalID := parts[3]
			handleDecideProposal(w, r, traceID, projectID, proposalID, "rejected")
			return
		}

		// /proposals/{proposal_id}/publish  (v10.1)
		if len(parts) == 5 && parts[4] == "publish" && r.Method == http.MethodPost {
			proposalID := parts[3]
			handlePublishProposal(w, r, traceID, projectID, proposalID)
			return
		}

		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
	})

	// root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"service":  serviceName,
			"message":  "agentsvc up",
			"trace_id": traceID,
		})
	})

	addr := "0.0.0.0:" + port
	log.Printf("[%s] listening on %s", serviceName, addr)
	if err := http.ListenAndServe(addr, withLogging(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// ------------------------------
// v10.1 publish: agentsvc -> govsvc
// ------------------------------
func handlePublishProposal(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	var req PublishProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
		return
	}
	if req.PublishedBy == "" || req.PublishReason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "missing_fields",
			"detail": "published_by and publish_reason are required",
			"trace_id": traceID,
		})
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "publink-" + newTraceID()
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var policySetID string
	var status string
	err = conn.QueryRow(ctx, `
	  SELECT policy_set_id::text, status
	    FROM agent_v10.routing_proposals_v10
	   WHERE project_id=$1 AND id=$2
	`, projectID, proposalID).Scan(&policySetID, &status)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	if status != "approved" {
		writeJSON(w, http.StatusConflict, map[string]any{"error":"proposal_not_approved","current_status":status,"trace_id":traceID})
		return
	}

	// call govsvc publish
	govBase := os.Getenv("GOVSVC_BASE_URL")
	if govBase == "" {
		govBase = "http://govsvc:9012"
	}
	url := govBase + "/v1/policies/sets/" + policySetID + "/publish"

	payload := map[string]any{
		"project_id":      projectID,
		"published_by":    req.PublishedBy,
		"publish_reason":  req.PublishReason,
		"confirm":         true,
		"idempotency_key": req.IdempotencyKey,
	}
	bodyBytes, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(traceHeader, traceID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
			projectID, proposalID, "review_required")
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "review_required",
			"error": "govsvc_publish_failed",
			"detail": err.Error(),
			"trace_id": traceID,
		})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
			projectID, proposalID, "review_required")
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "review_required",
			"error": "govsvc_publish_non2xx",
			"http_status": resp.StatusCode,
			"body": string(respBody),
			"trace_id": traceID,
		})
		return
	}

	var govRes GovPublishResponse
	_ = json.Unmarshal(respBody, &govRes)
	if govRes.PolicyVersionID == "" || govRes.PublicationID == "" {
		_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
			projectID, proposalID, "review_required")
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "review_required",
			"error": "govsvc_publish_missing_fields",
			"body": string(respBody),
			"trace_id": traceID,
		})
		return
	}

	// mark published
	_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
		projectID, proposalID, "published")

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "published",
		"proposal_id": proposalID,
		"policy_set_id": policySetID,
		"policy_version_id": govRes.PolicyVersionID,
		"publication_id": govRes.PublicationID,
		"compiled_policy_evidence_ref": govRes.CompiledPolicyEvidenceRef,
		"compiled_policy_checksum": govRes.CompiledPolicyChecksum,
		"trace_id": traceID,
	})
}

// ------------------------------
// approve/reject with FINAL LOCK
// ------------------------------
func handleDecideProposal(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID, newStatus string) {
	var req DecideProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
		return
	}
	if strings.TrimSpace(req.ReasonSummary) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_reason_summary", "trace_id": traceID})
		return
	}
	if req.IdempotencyKey == "" {
		prefix := "ap-"
		if newStatus == "rejected" {
			prefix = "rj-"
		}
		req.IdempotencyKey = prefix + newTraceID()
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// FINAL LOCK
	var current string
	err = conn.QueryRow(ctx, `
	  SELECT status FROM agent_v10.routing_proposals_v10
	   WHERE project_id=$1 AND id=$2
	`, projectID, proposalID).Scan(&current)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"db_call_failed","detail":err.Error(),"trace_id":traceID})
		return
	}
	if current == "approved" || current == "rejected" || current == "published" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "final_state_locked",
			"current_status": current,
			"trace_id": traceID,
		})
		return
	}

	evidenceDir := os.Getenv("AGENTSVC_EVIDENCE_DIR")
	if evidenceDir == "" {
		evidenceDir = defaultEvidenceDir
	}
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_dir_unavailable", "detail": err.Error(), "trace_id": traceID})
		return
	}

	payload := map[string]any{
		"proposal_id":    proposalID,
		"decision":       newStatus,
		"reason_summary": req.ReasonSummary,
		"decided_at":     time.Now().Format(time.RFC3339Nano),
		"service":        "agentsvc",
	}
	b, _ := json.Marshal(payload)
	sha := sha256Hex(b)
	p := filepath.Join(evidenceDir, "proposal_decision_"+sha+".json")
	if err := writeFileIfNotExists(p, b); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	reasonRef, _, err := evidenceRegisterV18(
		ctx, conn,
		projectID, traceID,
		"system", "agentsvc",
		"text", "application/json",
		"generated", p,
		sha, int64(len(b)),
		"ja", "standard",
		nil,
		"evi-"+req.IdempotencyKey+"-decision",
	)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
		projectID, proposalID, newStatus)

	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id":         proposalID,
		"status":              newStatus,
		"reason_evidence_ref": reasonRef,
		"trace_id":            traceID,
	})
}

// ------------------------------
// evaluate/get evaluation (unchanged logic)
// ------------------------------
func handleEvaluateProposal(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	var req EvaluateProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "eval-" + newTraceID()
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
		projectID, proposalID, "evaluating")

	evidenceDir := os.Getenv("AGENTSVC_EVIDENCE_DIR")
	if evidenceDir == "" {
		evidenceDir = defaultEvidenceDir
	}
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_dir_unavailable", "detail": err.Error(), "trace_id": traceID})
		return
	}

	dataset := map[string]any{
		"type":        "offline_replay_stub",
		"proposal_id":  proposalID,
		"generated_at": time.Now().Format(time.RFC3339Nano),
		"note":        "stub dataset; replace with real replay set later",
	}
	dsBytes, _ := json.Marshal(dataset)
	dsSha := sha256Hex(dsBytes)
	dsPath := filepath.Join(evidenceDir, "eval_dataset_"+dsSha+".json")
	if err := writeFileIfNotExists(dsPath, dsBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	datasetRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "agentsvc", "text", "application/json", "generated", dsPath, dsSha, int64(len(dsBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-dataset")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	metrics := map[string]any{
		"before": map[string]any{"success_rate": 0.98, "cost": 1.0, "p95_ms": 120},
		"after":  map[string]any{"success_rate": 0.981, "cost": 1.0, "p95_ms": 120},
		"delta":  map[string]any{"success_rate": 0.001, "cost": 0.0, "p95_ms": 0},
	}
	mtBytes, _ := json.Marshal(metrics)
	mtSha := sha256Hex(mtBytes)
	mtPath := filepath.Join(evidenceDir, "eval_metrics_"+mtSha+".json")
	if err := writeFileIfNotExists(mtPath, mtBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	metricsRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "agentsvc", "text", "application/json", "generated", mtPath, mtSha, int64(len(mtBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-metrics")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	metricsSummary := map[string]any{
		"before_success_rate": 0.98,
		"after_success_rate":  0.981,
		"before_cost":         1.0,
		"after_cost":          1.0,
		"before_p95_ms":       120,
		"after_p95_ms":        120,
	}
	guardSummary := map[string]any{
		"guard_pass": true,
		"failed_rules": []any{},
		"thresholds_snapshot": map[string]any{
			"success_rate_min_delta": -0.003,
		},
	}
	msJSON, _ := json.Marshal(metricsSummary)
	gsJSON, _ := json.Marshal(guardSummary)

	var evalID *string
	err = conn.QueryRow(ctx, `
		SELECT agent_v10.evaluation_create_v10(
			$1::varchar,
			$2::uuid,
			$3::varchar,
			$4::uuid,
			$5::uuid,
			$6::jsonb,
			$7::jsonb,
			$8::varchar,
			$9::varchar
		)::text
	`, projectID, proposalID, "offline_replay", datasetRef, metricsRef, msJSON, gsJSON, "succeeded", traceID).Scan(&evalID)

	if err != nil || evalID == nil || *evalID == "" {
		_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
			projectID, proposalID, "review_required")
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": errString(err), "trace_id": traceID})
		return
	}

	_, _ = conn.Exec(ctx, `SELECT agent_v10.proposal_set_status_v10($1::varchar,$2::uuid,$3::varchar)`,
		projectID, proposalID, "ready_for_review")

	writeJSON(w, http.StatusOK, map[string]any{
		"evaluation_id": *evalID,
		"proposal_id": proposalID,
		"status": "succeeded",
		"dataset_evidence_ref": datasetRef,
		"metrics_evidence_ref": metricsRef,
		"trace_id": traceID,
	})
}

func handleGetEvaluation(w http.ResponseWriter, r *http.Request, traceID, projectID, evalID string) {
	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := conn.QueryRow(ctx, `
		SELECT id::text, proposal_id::text, evaluation_type,
		       dataset_evidence_ref::text, metrics_evidence_ref::text,
		       metrics_summary, guard_summary, status, trace_id,
		       started_at, finished_at, created_at
		  FROM agent_v10.evaluation_get_v10($1::varchar, $2::uuid)
	`, projectID, evalID)

	var (
		id string
		proposalID string
		evalType string
		datasetRef string
		metricsRef string
		metricsBytes []byte
		guardBytes []byte
		status string
		trace string
		started *time.Time
		finished *time.Time
		created time.Time
	)
	if err := row.Scan(&id, &proposalID, &evalType, &datasetRef, &metricsRef, &metricsBytes, &guardBytes, &status, &trace, &started, &finished, &created); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	metrics := map[string]any{}
	guard := map[string]any{}
	_ = json.Unmarshal(metricsBytes, &metrics)
	_ = json.Unmarshal(guardBytes, &guard)

	writeJSON(w, http.StatusOK, map[string]any{
		"evaluation": map[string]any{
			"id": id,
			"proposal_id": proposalID,
			"evaluation_type": evalType,
			"dataset_evidence_ref": datasetRef,
			"metrics_evidence_ref": metricsRef,
			"metrics_summary": metrics,
			"guard_summary": guard,
			"status": status,
			"trace_id": trace,
			"started_at": started,
			"finished_at": finished,
			"created_at": created,
		},
		"trace_id": traceID,
	})
}

// create/list/get (same as your working implementation)
func handleCreateProposal(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	var req CreateProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
		return
	}
	if req.PolicySetID == "" || req.ProposalType == "" || req.RiskLevel == "" || req.RationaleSummary == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "missing_fields",
			"detail": "policy_set_id, proposal_type, risk_level, rationale_summary are required",
			"trace_id": traceID,
		})
		return
	}
	if req.ChangeSet == nil || req.Rationale == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "missing_fields",
			"detail": "change_set and rationale are required",
			"trace_id": traceID,
		})
		return
	}
	if len(req.RationaleSummary) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "rationale_summary_too_long", "trace_id": traceID})
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "prop-" + newTraceID()
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	evidenceDir := os.Getenv("AGENTSVC_EVIDENCE_DIR")
	if evidenceDir == "" {
		evidenceDir = defaultEvidenceDir
	}
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_dir_unavailable", "detail": err.Error(), "trace_id": traceID})
		return
	}

	changeSetBytes, _ := json.Marshal(req.ChangeSet)
	changeSha := sha256Hex(changeSetBytes)
	changePath := filepath.Join(evidenceDir, "routing_change_set_"+changeSha+".json")
	if err := writeFileIfNotExists(changePath, changeSetBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	changeRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "agentsvc", "text", "application/json", "generated", changePath, changeSha, int64(len(changeSetBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-changeset")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	rationaleBytes, _ := json.Marshal(req.Rationale)
	rationaleSha := sha256Hex(rationaleBytes)
	rationalePath := filepath.Join(evidenceDir, "routing_rationale_"+rationaleSha+".json")
	if err := writeFileIfNotExists(rationalePath, rationaleBytes); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	rationaleRef, _, err := evidenceRegisterV18(ctx, conn, projectID, traceID, "system", "agentsvc", "text", "application/json", "generated", rationalePath, rationaleSha, int64(len(rationaleBytes)), "ja", "standard", nil, "evi-"+req.IdempotencyKey+"-rationale")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}

	impactSummary := map[string]any{}
	if req.ImpactSummary != nil {
		impactSummary = req.ImpactSummary
	}
	impactJSON, _ := json.Marshal(impactSummary)

	var proposalID *string
	err = conn.QueryRow(ctx, `
		SELECT agent_v10.proposal_create_v10(
			$1::varchar,
			$2::uuid,
			$3::uuid,
			$4::varchar,
			$5::varchar,
			$6::uuid,
			$7::varchar,
			$8::uuid,
			$9::jsonb,
			$10::varchar,
			$11::varchar,
			$12::varchar,
			$13::text
		)::text
	`,
		projectID,
		req.PolicySetID,
		nullUUID(req.PolicyVersionBase),
		req.ProposalType,
		req.RiskLevel,
		changeRef,
		req.RationaleSummary,
		rationaleRef,
		impactJSON,
		"system",
		"agentsvc",
		traceID,
		req.IdempotencyKey,
	).Scan(&proposalID)

	if err != nil || proposalID == nil || *proposalID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "db_call_failed",
			"detail": errString(err),
			"trace_id": traceID,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id": *proposalID,
		"change_set_evidence_ref": changeRef,
		"rationale_evidence_ref": rationaleRef,
		"trace_id": traceID,
	})
}

func handleListProposals(w http.ResponseWriter, r *http.Request, traceID, projectID string) {
	status := r.URL.Query().Get("status")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

	var statusArg any = nil
	if status != "" {
		statusArg = status
	}

	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := conn.Query(ctx, `
		SELECT id::text, project_id, policy_set_id::text, COALESCE(policy_version_base::text,''),
		       proposal_type, risk_level, change_set_evidence_ref::text, rationale_summary, rationale_evidence_ref::text,
		       impact_summary, status, created_by_type, COALESCE(created_by_id,''),
		       created_at, updated_at
		  FROM agent_v10.proposal_list_v10($1::varchar, $2::varchar, $3::int, $4::int)
	`, projectID, statusArg, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer rows.Close()

	type item struct {
		ID                string         `json:"id"`
		ProjectID         string         `json:"project_id"`
		PolicySetID       string         `json:"policy_set_id"`
		PolicyVersionBase string         `json:"policy_version_base"`
		ProposalType      string         `json:"proposal_type"`
		RiskLevel         string         `json:"risk_level"`
		ChangeSetEvidence string         `json:"change_set_evidence_ref"`
		RationaleSummary  string         `json:"rationale_summary"`
		RationaleEvidence string         `json:"rationale_evidence_ref"`
		ImpactSummary     map[string]any `json:"impact_summary"`
		Status            string         `json:"status"`
		CreatedByType     string         `json:"created_by_type"`
		CreatedByID       string         `json:"created_by_id"`
		CreatedAt         time.Time      `json:"created_at"`
		UpdatedAt         time.Time      `json:"updated_at"`
	}

	var out []item
	for rows.Next() {
		var it item
		var impactBytes []byte
		var policyBase string
		var createdByID string
		if err := rows.Scan(
			&it.ID, &it.ProjectID, &it.PolicySetID, &policyBase,
			&it.ProposalType, &it.RiskLevel, &it.ChangeSetEvidence, &it.RationaleSummary, &it.RationaleEvidence,
			&impactBytes, &it.Status, &it.CreatedByType, &createdByID,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scan_failed", "detail": err.Error(), "trace_id": traceID})
			return
		}
		it.PolicyVersionBase = policyBase
		it.CreatedByID = createdByID
		it.ImpactSummary = map[string]any{}
		_ = json.Unmarshal(impactBytes, &it.ImpactSummary)
		out = append(out, it)
	}

	writeJSON(w, http.StatusOK, map[string]any{"proposals": out, "trace_id": traceID})
}

func handleGetProposal(w http.ResponseWriter, r *http.Request, traceID, projectID, proposalID string) {
	conn, err := openDB(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	defer conn.Close(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := conn.QueryRow(ctx, `
		SELECT id::text, project_id, policy_set_id::text, COALESCE(policy_version_base::text,''),
		       proposal_type, risk_level, change_set_evidence_ref::text, rationale_summary, rationale_evidence_ref::text,
		       impact_summary, status, created_by_type, COALESCE(created_by_id,''),
		       created_at, updated_at
		  FROM agent_v10.proposal_get_v10($1::varchar, $2::uuid)
	`, projectID, proposalID)

	type resp struct {
		ID                string         `json:"id"`
		ProjectID         string         `json:"project_id"`
		PolicySetID       string         `json:"policy_set_id"`
		PolicyVersionBase string         `json:"policy_version_base"`
		ProposalType      string         `json:"proposal_type"`
		RiskLevel         string         `json:"risk_level"`
		ChangeSetEvidence string         `json:"change_set_evidence_ref"`
		RationaleSummary  string         `json:"rationale_summary"`
		RationaleEvidence string         `json:"rationale_evidence_ref"`
		ImpactSummary     map[string]any `json:"impact_summary"`
		Status            string         `json:"status"`
		CreatedByType     string         `json:"created_by_type"`
		CreatedByID       string         `json:"created_by_id"`
		CreatedAt         time.Time      `json:"created_at"`
		UpdatedAt         time.Time      `json:"updated_at"`
	}

	var out resp
	var impactBytes []byte
	var policyBase string
	var createdByID string
	if err := row.Scan(
		&out.ID, &out.ProjectID, &out.PolicySetID, &policyBase,
		&out.ProposalType, &out.RiskLevel, &out.ChangeSetEvidence, &out.RationaleSummary, &out.RationaleEvidence,
		&impactBytes, &out.Status, &out.CreatedByType, &createdByID,
		&out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": err.Error(), "trace_id": traceID})
		return
	}
	out.PolicyVersionBase = policyBase
	out.CreatedByID = createdByID
	out.ImpactSummary = map[string]any{}
	_ = json.Unmarshal(impactBytes, &out.ImpactSummary)

	writeJSON(w, http.StatusOK, map[string]any{"proposal": out, "trace_id": traceID})
}

// evidence_register_v18 wrapper
func evidenceRegisterV18(
	ctx context.Context,
	conn *pgx.Conn,
	projectID string,
	traceID string,
	createdByType string,
	createdByID string,
	mediaType string,
	mimeType string,
	sourceKind string,
	sourceURI string,
	contentSha256 string,
	contentLen int64,
	language string,
	retentionPolicy string,
	expiresAtUTC any,
	idempotencyKey string,
) (string, bool, error) {
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
	`,
		projectID, traceID,
		createdByType, createdByID,
		mediaType, mimeType,
		sourceKind, sourceURI,
		contentSha256, contentLen,
		language, retentionPolicy,
		expiresAtUTC,
		idempotencyKey,
	).Scan(&evidenceRef, &found)
	if err != nil {
		return "", false, err
	}
	return evidenceRef, found, nil
}

func openDB(parent context.Context) (*pgx.Conn, error) {
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" {
		return nil, errors.New("AK_DB_DSN is empty")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
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
	if v == "" {
		v = newTraceID()
	}
	w.Header().Set(traceHeader, v)
	return v
}

func newTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writeFileIfNotExists(path string, b []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}