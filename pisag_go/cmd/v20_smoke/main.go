package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	dbURL := mustEnv("DATABASE_URL")
	db, err := postgres.Open(dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectID := firstNonEmpty(os.Getenv("PROJECT_ID"), mustQueryString(ctx, db, `SELECT id FROM public.projects ORDER BY id LIMIT 1`))
	evidenceID := firstNonZeroInt64(parseEnvInt64("EVIDENCE_ASSET_ID"), mustQueryInt64(ctx, db, `SELECT id FROM public.evidence_assets ORDER BY id LIMIT 1`))

	approvedBy := strings.TrimSpace(os.Getenv("APPROVED_BY_USER_ID"))
	if approvedBy == "" {
		approvedBy = "smoke-user"
	}

	log.Printf("[v20_smoke] project_id=%s evidence_asset_id=%d approved_by=%s", projectID, evidenceID, approvedBy)

	// ★ v13 repo injection
	v13repo := postgres.NewV13Repository(db)

	// ---------------------------------------------------------------------
	// Create a "root" run
	// ---------------------------------------------------------------------
	rootRunID, rootTraceID := createRun(ctx, db, projectID, "v20_smoke_root")
	log.Printf("[v20_smoke] root run created: run_id=%s trace_id=%s", rootRunID, rootTraceID)

	// ---------------------------------------------------------------------
	// 1) Create Incident (EXECUTE ONLY fn)
	// ---------------------------------------------------------------------
	windowStart := time.Now().UTC().Add(-30 * time.Minute)
	windowEnd := time.Now().UTC()

	incidentKey := sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s",
		projectID,
		"slo_breach",
		rootTraceID,
		windowStart.Format(time.RFC3339Nano),
		windowEnd.Format(time.RFC3339Nano),
	))

	incidentID, found := incidentCreateV20(ctx, db, incidentCreateArgs{
		projectID:                 projectID,
		incidentKey:               incidentKey,
		status:                    "open",
		severity:                  "P2",
		incidentType:              "slo_breach",
		rootTraceID:               rootTraceID,
		rootRunID:                 rootRunID,
		detectedBy:                "slo",
		detectedAtUTC:             time.Now().UTC(),
		incidentSummaryEvidenceID: evidenceID,
		primaryEvidenceID:         0,
		ownerUserID:               "",
		idempotencyKey:            "idem_incident_" + shortRandHex(8),
	})
	log.Printf("[v20_smoke] incident_create_v20: incident_id=%d found_existing=%v", incidentID, found)

	// ---------------------------------------------------------------------
	// 2) Create Proposal (status forced to needs_review)
	// ---------------------------------------------------------------------
	primaryTargetKey := "target:run:" + rootRunID
	proposalKey := sha256Hex(fmt.Sprintf("%s|%d|%s|%s|%s",
		projectID,
		incidentID,
		"replay_run",
		primaryTargetKey,
		time.Now().UTC().Truncate(10*time.Minute).Format(time.RFC3339Nano),
	))

	proposalID, foundProp := proposalCreateV20(ctx, db, proposalCreateArgs{
		projectID:         projectID,
		incidentID:        incidentID,
		proposalKey:       proposalKey,
		proposalType:      "replay_run",
		riskLevel:         "medium",
		requiresApproval:  true,
		planEvidenceID:    evidenceID,
		impactEvidenceID:  evidenceID,
		primaryEvidenceID: 0,
		idempotencyKey:    "idem_proposal_" + shortRandHex(8),
	})
	log.Printf("[v20_smoke] proposal_create_v20: proposal_id=%d found_existing=%v (expected status=needs_review)", proposalID, foundProp)

	// ---------------------------------------------------------------------
	// 3) Ledger approval flow (v4.7): request -> decide approve
	// ---------------------------------------------------------------------
	approvalRepo := postgres.NewApprovalRepository(db)

	commitID, commitTraceID, err := pickAnyPublishCommit(ctx, db, projectID)
	if err != nil {
		log.Fatalf("[v20_smoke] cannot proceed: %v", err)
	}
	log.Printf("[v20_smoke] picked commit_id=%s commit_trace_id=%s", commitID, commitTraceID)

	reqUC := usecase.RequestApprovalUseCase{ApprovalRepo: approvalRepo}
	reqOut, err := reqUC.Handle(ctx, usecase.RequestApprovalInput{
		ProjectID:       projectID,
		CommitID:        commitID,
		TraceID:         rootTraceID,
		RequestedByType: run.ActorTypeSystem,
		RequestedByID:   ptr("v20_smoke"),
		Reason:          ptr("v20 proposal approve smoke"),
	})
	if err != nil {
		log.Fatalf("[v20_smoke] request_approval failed: %v", err)
	}
	log.Printf("[v20_smoke] request_approval: request_id=%s found_existing=%v status=%s", reqOut.RequestID, reqOut.FoundExisting, reqOut.Status)

	decUC := usecase.DecideApprovalUseCase{ApprovalRepo: approvalRepo}
	decOut, err := decUC.Handle(ctx, usecase.DecideApprovalInput{
		ProjectID:     projectID,
		RequestID:     reqOut.RequestID,
		TraceID:       rootTraceID,
		Decision:      run.DecisionApprove,
		DecidedByType: run.ActorTypeUser,
		DecidedByID:   ptr(approvedBy),
		Comment:       ptr("approve by smoke"),
	})
	if err != nil {
		log.Fatalf("[v20_smoke] decide_approval failed: %v", err)
	}
	log.Printf("[v20_smoke] decide_approval: request_id=%s decision=%s status=%s", decOut.RequestID, decOut.Decision, decOut.Status)

	// ---------------------------------------------------------------------
	// 4) v20 approve finalized ONLY if ledger approved (with v13 idempotency record)
	// ---------------------------------------------------------------------
	v20Approve := usecase.V20ProposalApproveUseCase{
		DB:           db,
		ApprovalRepo: approvalRepo,
		V13Repo:      v13repo, // ★注入
	}
	_, err = v20Approve.Handle(ctx, usecase.V20ProposalApproveInput{
		ProjectID:         projectID,
		TraceID:           rootTraceID,
		ProposalID:        proposalID,
		ApprovalRequestID: reqOut.RequestID,
		ApprovedByUserID:  ptr(approvedBy),
	})
	if err != nil {
		log.Fatalf("[v20_smoke] v20_proposal_approve failed: %v", err)
	}
	log.Printf("[v20_smoke] v20_proposal_approve: proposal_id=%d approved (ledger enforced)", proposalID)

	// ---------------------------------------------------------------------
	// 5) Apply = create remediation run + action_create_v20
	// ---------------------------------------------------------------------
	applyRunID, applyTraceID := createRun(ctx, db, projectID, "v20_smoke_apply")
	log.Printf("[v20_smoke] remediation run created: run_id=%s trace_id=%s", applyRunID, applyTraceID)

	actionKey := sha256Hex(fmt.Sprintf("%s|%d|%d", projectID, proposalID, 1))
	if err := actionCreateV20(ctx, db, projectID, proposalID, actionKey, applyRunID, "queued", evidenceID); err != nil {
		log.Fatalf("[v20_smoke] action_create_v20 failed: %v", err)
	}
	log.Printf("[v20_smoke] action_create_v20: proposal_id=%d action_key=%s run_id=%s status=queued", proposalID, actionKey, applyRunID)

	if strings.ToLower(os.Getenv("SKIP_AUDIT")) != "true" {
		auditID, auditFound, auditErr := auditAppendV18(ctx, db, auditAppendArgs{
			projectID:      projectID,
			traceID:        applyTraceID,
			runID:          "",
			action:         "v20_smoke_apply_queued",
			actorType:      "system",
			actorID:        "v20_smoke",
			targetType:     "proposal",
			targetID:       fmt.Sprintf("%d", proposalID),
			result:         "ok",
			reason:         "smoke",
			metaJSON:       `{}`,
			idempotencyKey: "idem_audit_" + shortRandHex(8),
		})
		if auditErr != nil {
			log.Printf("[v20_smoke] audit_event_append_v18 skipped/failed (non-fatal): %v", auditErr)
		} else {
			log.Printf("[v20_smoke] audit_event_append_v18: audit_event_id=%d found_existing=%v", auditID, auditFound)
		}
	}

	log.Printf("[v20_smoke] ✅ DONE")
}

func ptr(s string) *string { return &s }

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func firstNonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		return v
	}
	return fallback
}

func parseEnvInt64(k string) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("invalid %s: %v", k, err)
	}
	return n
}

func firstNonZeroInt64(v, fallback int64) int64 {
	if v != 0 {
		return v
	}
	return fallback
}

func mustQueryString(ctx context.Context, db *sql.DB, q string) string {
	var s string
	if err := db.QueryRowContext(ctx, q).Scan(&s); err != nil {
		log.Fatalf("query string failed: %v (sql=%s)", err, q)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		log.Fatalf("query returned empty string (sql=%s)", q)
	}
	return s
}

func mustQueryInt64(ctx context.Context, db *sql.DB, q string) int64 {
	var n int64
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		log.Fatalf("query int64 failed: %v (sql=%s)", err, q)
	}
	if n == 0 {
		log.Fatalf("query returned zero int64 (sql=%s)", q)
	}
	return n
}

func shortRandHex(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 8
	}
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func createRun(ctx context.Context, db *sql.DB, projectID, pipelineVersion string) (runID string, traceID string) {
	const q = `
WITH ins AS (
  INSERT INTO public.runs(project_id, pipeline_version, status)
  VALUES ($1, $2, 'running')
  RETURNING run_id::text AS run_id_text, trace_id::text AS trace_id_text
)
SELECT run_id_text, trace_id_text FROM ins;
`
	if err := db.QueryRowContext(ctx, q, projectID, pipelineVersion).Scan(&runID, &traceID); err != nil {
		log.Fatalf("createRun failed: %v", err)
	}
	return runID, traceID
}

// ---------------------------------------------------------------------
// v20 function callers
// ---------------------------------------------------------------------

type incidentCreateArgs struct {
	projectID                 string
	incidentKey               string
	status                    string
	severity                  string
	incidentType              string
	rootTraceID               string
	rootRunID                 string
	detectedBy                string
	detectedAtUTC             time.Time
	incidentSummaryEvidenceID int64
	primaryEvidenceID         int64
	ownerUserID               string
	idempotencyKey            string
}

func incidentCreateV20(ctx context.Context, db *sql.DB, a incidentCreateArgs) (incidentID int64, foundExisting bool) {
	const q = `
SELECT incident_id, found_existing
FROM public.incident_create_v20(
  $1,$2,$3,$4,$5,($6)::uuid,($7)::uuid,$8,$9,$10,NULLIF($11::bigint,0),NULLIF($12,''),NULLIF($13,'')
);`
	err := db.QueryRowContext(ctx, q,
		a.projectID, a.incidentKey, a.status, a.severity, a.incidentType,
		a.rootTraceID, a.rootRunID, a.detectedBy, a.detectedAtUTC,
		a.incidentSummaryEvidenceID, a.primaryEvidenceID, a.ownerUserID, a.idempotencyKey,
	).Scan(&incidentID, &foundExisting)
	if err != nil {
		log.Fatalf("incident_create_v20 failed: %v", err)
	}
	return incidentID, foundExisting
}

type proposalCreateArgs struct {
	projectID         string
	incidentID        int64
	proposalKey       string
	proposalType      string
	riskLevel         string
	requiresApproval  bool
	planEvidenceID    int64
	impactEvidenceID  int64
	primaryEvidenceID int64
	idempotencyKey    string
}

func proposalCreateV20(ctx context.Context, db *sql.DB, a proposalCreateArgs) (proposalID int64, foundExisting bool) {
	const q = `
SELECT proposal_id, found_existing
FROM public.proposal_create_v20(
  $1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9::bigint,0),NULLIF($10,'')
);`
	err := db.QueryRowContext(ctx, q,
		a.projectID, a.incidentID, a.proposalKey, a.proposalType, a.riskLevel,
		a.requiresApproval, a.planEvidenceID, a.impactEvidenceID, a.primaryEvidenceID, a.idempotencyKey,
	).Scan(&proposalID, &foundExisting)
	if err != nil {
		log.Fatalf("proposal_create_v20 failed: %v", err)
	}
	return proposalID, foundExisting
}

func actionCreateV20(ctx context.Context, db *sql.DB, projectID string, proposalID int64, actionKey string, runID string, status string, evidenceID int64) error {
	const q = `SELECT public.action_create_v20($1,$2,$3,($4)::uuid,$5,$6);`
	_, err := db.ExecContext(ctx, q, projectID, proposalID, actionKey, runID, status, evidenceID)
	return err
}

// ---------------------------------------------------------------------
// pick a commit for v4.7 request approval
// ---------------------------------------------------------------------

func pickAnyPublishCommit(ctx context.Context, db *sql.DB, projectID string) (commitID string, traceID string, err error) {
	const q = `
SELECT commit_id::text, trace_id::text
FROM public.catalog_publish_commits
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT 1;
`
	err = db.QueryRowContext(ctx, q, projectID).Scan(&commitID, &traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.New("no catalog_publish_commits found for project; create one first or run publish_confirm_smoke")
		}
		return "", "", err
	}
	return commitID, traceID, nil
}

// ---------------------------------------------------------------------
// Optional audit append
// ---------------------------------------------------------------------

type auditAppendArgs struct {
	projectID      string
	traceID        string
	runID          string
	action         string
	actorType      string
	actorID        string
	targetType     string
	targetID       string
	result         string
	reason         string
	metaJSON       string
	idempotencyKey string
}

func auditAppendV18(ctx context.Context, db *sql.DB, a auditAppendArgs) (auditEventID int64, foundExisting bool, err error) {
	const q = `
SELECT audit_event_id, found_existing
FROM public.audit_event_append_v18(
  $1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,NULLIF($10,''),($11)::jsonb,NULLIF($12,'')
);`
	err = db.QueryRowContext(ctx, q,
		a.projectID, a.traceID, a.runID, a.action, a.actorType, a.actorID,
		a.targetType, a.targetID, a.result, a.reason, ifEmpty(a.metaJSON, `{}`), a.idempotencyKey,
	).Scan(&auditEventID, &foundExisting)
	return auditEventID, foundExisting, err
}

func ifEmpty(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	return s
}