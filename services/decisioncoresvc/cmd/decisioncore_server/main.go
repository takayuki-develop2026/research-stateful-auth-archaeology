package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/decisioncoresvc/internal/httpx"
	"example.com/decisioncoresvc/postgres"
)

type Server struct {
	db   *sql.DB
	v23  *postgres.V23Repo
	ev18 *postgres.EvidenceV18Bridge
}

func main() {
	port := env("DECISIONCORESVC_PORT", "9023")
	dsn := mustEnv("AK_DB_DSN")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	s := &Server{
		db:   db,
		v23:  postgres.NewV23Repo(db),
		ev18: postgres.NewEvidenceV18Bridge(db),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/", s.routeV1)

	log.Printf("decisioncoresvc listening on :%s", port)
	if err := http.ListenAndServe(":"+port, withTrace(mux)); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func withTrace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace := httpx.SanitizeTraceID(r.Header.Get(httpx.TraceHeader))
		if trace == "" {
			trace = httpx.NewTraceID()
		}
		ctx := context.WithValue(r.Context(), ctxKeyTraceID{}, trace)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type ctxKeyTraceID struct{}

func traceIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTraceID{}).(string)
	return v
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "now": time.Now().Format(time.RFC3339)}, traceIDFrom(r.Context()))
}

func (s *Server) routeV1(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/v1/")
	parts := strings.Split(p, "/")
	if len(parts) < 3 || parts[0] != "projects" {
		httpx.WriteError(w, 404, "not_found", "unknown route", traceIDFrom(r.Context()))
		return
	}
	projectID := parts[1]

	if len(parts) == 4 && parts[2] == "policy" && parts[3] == "evaluate" && r.Method == "POST" {
		s.handlePolicyEvaluate(w, r, projectID)
		return
	}
	if len(parts) == 3 && parts[2] == "decisions" && r.Method == "POST" {
		s.handleDecisionPropose(w, r, projectID)
		return
	}
	if len(parts) == 5 && parts[2] == "decisions" && parts[4] == "approve" && r.Method == "POST" {
		s.handleDecisionApprove(w, r, projectID, parts[3])
		return
	}
	if len(parts) == 5 && parts[2] == "decisions" && parts[4] == "apply" && r.Method == "POST" {
		s.handleDecisionApply(w, r, projectID, parts[3])
		return
	}

	httpx.WriteError(w, 404, "not_found", "unknown route", traceIDFrom(r.Context()))
}

// ---------------- handlers ----------------

type PolicyEvaluateRequest struct {
	RunID                 string `json:"run_id"`
	PolicyVersionStr      string `json:"policy_version_str"`
	PipelineVersion       string `json:"pipeline_version"`
	InputsEvidenceAssetID int64  `json:"inputs_evidence_asset_id"`
	ReasonEvidenceAssetID int64  `json:"reason_evidence_asset_id"`
	ObligationsEvidenceAssetID int64 `json:"obligations_evidence_asset_id"`
}

func (s *Server) handlePolicyEvaluate(w http.ResponseWriter, r *http.Request, projectID string) {
	trace := traceIDFrom(r.Context())
	var req PolicyEvaluateRequest

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, 400, "bad_request", "invalid json: "+err.Error(), trace)
		return
	}

	// Robust trimming (prevents newline-in-string issues)
	req.RunID = strings.TrimSpace(req.RunID)
	req.PolicyVersionStr = strings.TrimSpace(req.PolicyVersionStr)
	req.PipelineVersion = strings.TrimSpace(req.PipelineVersion)

	if req.RunID == "" || req.PolicyVersionStr == "" || req.PipelineVersion == "" ||
		req.InputsEvidenceAssetID == 0 || req.ReasonEvidenceAssetID == 0 || req.ObligationsEvidenceAssetID == 0 {
		httpx.WriteError(w, 400, "bad_request", "missing required fields", trace)
		return
	}

	inputHash := sha256hex(fmt.Sprintf("inputs:%d|trace:%s", req.InputsEvidenceAssetID, trace))

	id, err := s.v23.PolicyEvaluationUpsert(r.Context(), postgres.PolicyEvalUpsertInput{
		ProjectID:        projectID,
		TraceID:          trace,
		RunID:            req.RunID,
		PolicyVersion:    req.PolicyVersionStr,
		PipelineVersion:  req.PipelineVersion,
		InputHash:        inputHash,
		PdpMode:          "local",
		Result:           "allow",
		ScoreNullable:    nil,
		ReasonAssetID:    req.ReasonEvidenceAssetID,
		ObligAssetID:     req.ObligationsEvidenceAssetID,
		ProposalAssetID:  0,
		PolicyDecisionID: 0,
	})
	if err != nil {
		httpx.WriteError(w, 500, "db_error", err.Error(), trace)
		return
	}

	httpx.WriteJSON(w, 200, map[string]any{
		"trace_id":             trace,
		"policy_evaluation_id": id,
		"result":               "allow",
		"input_hash":           inputHash,
	}, trace)
}

type DecisionProposeRequest struct {
	RunID              string `json:"run_id"`
	PolicyEvaluationID int64  `json:"policy_evaluation_id"`
	SubjectType        string `json:"subject_type"`
	SubjectID          string `json:"subject_id"`
	DecisionScope      string `json:"decision_scope"`

	PolicyVersionStr string `json:"policy_version_str"`
	PipelineVersion  string `json:"pipeline_version"`
	InputHash        string `json:"input_hash"`

	InputsEvidenceAssetID      int64 `json:"inputs_evidence_asset_id"`
	ObligationsEvidenceAssetID int64 `json:"obligations_evidence_asset_id"`
}

func (s *Server) handleDecisionPropose(w http.ResponseWriter, r *http.Request, projectID string) {
	trace := traceIDFrom(r.Context())
	var req DecisionProposeRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, 400, "bad_request", "invalid json: "+err.Error(), trace)
		return
	}

	req.RunID = strings.TrimSpace(req.RunID)
	req.SubjectType = strings.TrimSpace(req.SubjectType)
	req.SubjectID = strings.TrimSpace(req.SubjectID)
	req.PolicyVersionStr = strings.TrimSpace(req.PolicyVersionStr)
	req.PipelineVersion = strings.TrimSpace(req.PipelineVersion)
	req.InputHash = strings.TrimSpace(req.InputHash)
	req.DecisionScope = strings.TrimSpace(req.DecisionScope)
	if req.DecisionScope == "" {
		req.DecisionScope = "managed"
	}

	if req.RunID == "" || req.SubjectType == "" || req.SubjectID == "" ||
		req.PolicyVersionStr == "" || req.PipelineVersion == "" || req.InputHash == "" ||
		req.InputsEvidenceAssetID == 0 || req.ObligationsEvidenceAssetID == 0 {
		httpx.WriteError(w, 400, "bad_request", "missing required fields", trace)
		return
	}

	decisionKey := sha256hex(projectID + "|" + req.SubjectType + "|" + req.SubjectID + "|" + req.PolicyVersionStr + "|" + req.PipelineVersion + "|" + req.InputHash + "|propose")

	id, err := s.v23.DecisionPropose(r.Context(), postgres.DecisionProposeInput{
		ProjectID:             projectID,
		TraceID:               trace,
		RunID:                 req.RunID,
		SubjectType:           req.SubjectType,
		SubjectID:             req.SubjectID,
		SubjectOwnerProjectID: projectID,
		DecisionKey:           decisionKey,
		DecisionScope:         req.DecisionScope,
		PolicyVersion:         req.PolicyVersionStr,
		PipelineVersion:       req.PipelineVersion,
		InputHash:             req.InputHash,
		InputsAssetID:         req.InputsEvidenceAssetID,
		ProposalAssetID:       0,
		ObligationsAssetID:    req.ObligationsEvidenceAssetID,
		PolicyEvaluationID:    req.PolicyEvaluationID,
		DecidedByType:         "system",
		DecidedByID:           "decisioncoresvc",
		InitialStatus:         "proposed",
		ExpiresAtNullable:     nil,
	})
	if err != nil {
		httpx.WriteError(w, 500, "db_error", err.Error(), trace)
		return
	}

	httpx.WriteJSON(w, 200, map[string]any{
		"trace_id":     trace,
		"decision_id":  id,
		"decision_key": decisionKey,
		"status":       "proposed",
	}, trace)
}

type DecisionApproveRequest struct {
	DecidedByID string `json:"decided_by_id"`
}

func (s *Server) handleDecisionApprove(w http.ResponseWriter, r *http.Request, projectID, decisionIDStr string) {
	trace := traceIDFrom(r.Context())
	decisionID, err := strconv.ParseInt(decisionIDStr, 10, 64)
	if err != nil || decisionID <= 0 {
		httpx.WriteError(w, 400, "bad_request", "invalid decision id", trace)
		return
	}
	var req DecisionApproveRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.DecidedByID = strings.TrimSpace(req.DecidedByID)
	if req.DecidedByID == "" {
		req.DecidedByID = "human"
	}
	if err := s.v23.DecisionApprove(r.Context(), decisionID, projectID, "human", req.DecidedByID, 0); err != nil {
		httpx.WriteError(w, 500, "db_error", err.Error(), trace)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"trace_id": trace, "decision_id": decisionID, "status": "approved"}, trace)
}

type DecisionApplyRequest struct {
	RunID                string `json:"run_id"`
	ActionType           string `json:"action_type"`
	ActionScope          string `json:"action_scope"`
	TargetEvidenceAssetID int64 `json:"target_evidence_asset_id"`
	PlanEvidenceAssetID   int64 `json:"plan_evidence_asset_id"`
	BudgetCurrency       string `json:"budget_currency"`
	BudgetEstimate       int64  `json:"budget_estimate_amount"`
}

func (s *Server) handleDecisionApply(w http.ResponseWriter, r *http.Request, projectID, decisionIDStr string) {
	trace := traceIDFrom(r.Context())

	decisionID, err := strconv.ParseInt(decisionIDStr, 10, 64)
	if err != nil || decisionID <= 0 {
		httpx.WriteError(w, 400, "bad_request", "invalid decision id", trace)
		return
	}

	var req DecisionApplyRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, 400, "bad_request", "invalid json: "+err.Error(), trace)
		return
	}

	req.RunID = strings.TrimSpace(req.RunID)
	req.ActionType = strings.TrimSpace(req.ActionType)
	req.ActionScope = strings.TrimSpace(req.ActionScope)
	req.BudgetCurrency = strings.TrimSpace(req.BudgetCurrency)

	if req.RunID == "" || req.TargetEvidenceAssetID == 0 || req.PlanEvidenceAssetID == 0 {
		httpx.WriteError(w, 400, "bad_request", "missing required fields", trace)
		return
	}
	if req.ActionType == "" {
		req.ActionType = "publish_http"
	}
	if req.ActionScope == "" {
		req.ActionScope = "managed"
	}
	if req.BudgetCurrency == "" {
		req.BudgetCurrency = "usd_micros"
	}
	if req.BudgetEstimate <= 0 {
		req.BudgetEstimate = 1000
	}

	// --------- NEW: gate by decision status (exec-only compatible) ---------
	status, kind, err := s.v23.DecisionStatusGet(r.Context(), projectID, decisionID)
	if err != nil {
		httpx.WriteError(w, 500, "db_error", err.Error(), trace)
		return
	}
	if status == "" {
		// decision not found under this project
		httpx.WriteJSON(w, 200, map[string]any{
			"trace_id":    trace,
			"decision_id": decisionID,
			"decision_status": "not_found",
			"actions":     []any{},
		}, trace)
		return
	}
	if status != "approved" {
		// v23 rule: do NOT enqueue when not approved. throw禁止 → actions=[]
		httpx.WriteJSON(w, 200, map[string]any{
			"trace_id":        trace,
			"decision_id":     decisionID,
			"decision_status": status,
			"decision_kind":   kind,
			"actions":         []any{},
			"blocked_reason":  "decision_not_approved",
		}, trace)
		return
	}

	// --------- enqueue allowed ---------
	targetHash := sha256hex(fmt.Sprintf("target:%d", req.TargetEvidenceAssetID))
	actionKey := sha256hex(fmt.Sprintf("%s|%d|%s|%s|%s|%s", projectID, decisionID, req.ActionType, req.ActionScope, targetHash, "v23"))

	actionID, err := s.v23.ActionEnqueue(r.Context(), postgres.ActionEnqueueInput{
		ProjectID:        projectID,
		TraceID:          trace,
		RunID:            req.RunID,
		DecisionLedgerID: decisionID,
		ActionKey:        actionKey,
		ActionType:       req.ActionType,
		ActionScope:      req.ActionScope,
		TargetHash:       targetHash,
		TargetAssetID:    req.TargetEvidenceAssetID,
		PlanAssetID:      req.PlanEvidenceAssetID,
		BudgetCurrency:   req.BudgetCurrency,
		BudgetEstimate:   req.BudgetEstimate,
		InitialStatus:    "queued",
		ErrorAssetID:     0,
	})
	if err != nil {
		httpx.WriteError(w, 500, "db_error", err.Error(), trace)
		return
	}

	httpx.WriteJSON(w, 200, map[string]any{
		"trace_id":    trace,
		"decision_id": decisionID,
		"decision_status": status,
		"actions": []any{
			map[string]any{"action_id": actionID, "status": "queued"},
		},
	}, trace)
}

// ---- utils ----
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}
func env(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}