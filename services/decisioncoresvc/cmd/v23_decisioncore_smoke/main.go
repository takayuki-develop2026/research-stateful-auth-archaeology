package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/decisioncoresvc/postgres"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN")

	traceID := envString("AK_TRACE_ID", fmt.Sprintf("trc_v23_decisioncore_smoke_%d", time.Now().Unix()))
	actorType := envString("AK_ACTOR_TYPE", "system")
	actorID := envString("AK_ACTOR_ID", "seed:local|kawada")

	// project_id must exist in public.projects.id (string)
	projectID := mustPickProjectsID(ctx, dsn, envString("AK_PROJECT_ID", ""))

	policyVersionStr := envString("AK_POLICY_VERSION_STR", "v23")
	pipelineVersion := envString("AK_PIPELINE_VERSION", "v23")

	// ---- DB connect ----
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	log.Printf("project_id=%s trace_id=%s actor=%s:%s", projectID, traceID, actorType, actorID)

	// ---- run_id (create if missing) ----
	runID := envString("AK_RUN_ID", "")
	if strings.TrimSpace(runID) == "" {
		runID = mustCreateRun(ctx, db, projectID, pipelineVersion)
	}
	log.Printf("run_id=%s", runID)

	bridge := postgres.NewEvidenceV18Bridge(db)

	// ---------------------------------------------------------------------
	// 0) Create evidence_assets IDs via exec-only v18 register (no direct INSERT)
	// ---------------------------------------------------------------------
	inputsAssetID := mustRegisterEvidenceAssetIDViaV18(ctx, bridge, projectID, traceID, actorType, actorID,
		"decision_input", `{"note":"inputs for v23 smoke"}`, "idem_v23_inputs_"+traceID)
	reasonAssetID := mustRegisterEvidenceAssetIDViaV18(ctx, bridge, projectID, traceID, actorType, actorID,
		"decision_reason", `{"reason":"ok for smoke"}`, "idem_v23_reason_"+traceID)
	obligAssetID := mustRegisterEvidenceAssetIDViaV18(ctx, bridge, projectID, traceID, actorType, actorID,
		"decision_obligations", `{"require_approval":false}`, "idem_v23_oblig_"+traceID)

	targetAssetID := mustRegisterEvidenceAssetIDViaV18(ctx, bridge, projectID, traceID, actorType, actorID,
		"action_target", `{"endpoint":"https://example.invalid/publish","method":"POST"}`, "idem_v23_target_"+traceID)
	planAssetID := mustRegisterEvidenceAssetIDViaV18(ctx, bridge, projectID, traceID, actorType, actorID,
		"action_plan", `{"steps":["dryrun"],"rollback":"none"}`, "idem_v23_plan_"+traceID)

	log.Printf("evidence_assets(v18): inputs=%d reason=%d obligations=%d target=%d plan=%d",
		inputsAssetID, reasonAssetID, obligAssetID, targetAssetID, planAssetID)

	// ---------------------------------------------------------------------
	// 1) policy_evaluation_upsert_v23
	// ---------------------------------------------------------------------
	inputHash := sha256hex("inputs:" + fmt.Sprint(inputsAssetID) + "|trace:" + traceID)

	var policyEvalID int64
	err = db.QueryRowContext(ctx, `
SELECT policy_evaluation_upsert_v23(
  $1,$2,$3::uuid,$4,$5,$6,
  $7,$8,$9,
  $10,$11,$12,$13
)`,
		projectID,
		traceID,
		runID,
		policyVersionStr,
		pipelineVersion,
		inputHash,
		"local",
		"allow",
		sql.NullFloat64{},
		input64(reasonAssetID),
		input64(obligAssetID),
		input64(0),
		input64(0),
	).Scan(&policyEvalID)
	if err != nil {
		log.Fatalf("policy_evaluation_upsert_v23 failed: %v", err)
	}
	log.Printf("policy_evaluation: id=%d input_hash=%s", policyEvalID, inputHash)

	// ---------------------------------------------------------------------
	// 2) decision_propose_v23
	// ---------------------------------------------------------------------
	subjectType := "catalog_source"
	subjectID := "src_smoke_001"
	subjectOwnerProjectID := projectID
	decisionKind := "propose"

	decisionKey := sha256hex(projectID + "|" + subjectType + "|" + subjectID + "|" + policyVersionStr + "|" + pipelineVersion + "|" + inputHash + "|" + decisionKind)
	decisionScope := "managed"

	var decisionID int64
	err = db.QueryRowContext(ctx, `
SELECT decision_propose_v23(
  $1,$2,$3::uuid,
  $4,$5,$6,
  $7,$8,
  $9,$10,$11,
  $12,$13,$14,
  $15,
  $16,$17,
  $18,$19
)`,
		projectID,
		traceID,
		runID,
		subjectType,
		subjectID,
		subjectOwnerProjectID,
		decisionKey,
		decisionScope,
		policyVersionStr,
		pipelineVersion,
		inputHash,
		input64(inputsAssetID),
		input64(0),
		input64(obligAssetID),
		policyEvalID,
		actorType,
		actorID,
		"proposed",
		sql.NullTime{},
	).Scan(&decisionID)
	if err != nil {
		log.Fatalf("decision_propose_v23 failed: %v", err)
	}
	log.Printf("decision_proposed: id=%d decision_key=%s", decisionID, decisionKey)

	// ---------------------------------------------------------------------
	// 3) decision_approve_v23 (DB SoT verification)
	// ---------------------------------------------------------------------
	var curProjectID, curStatus, curKind string
	_ = db.QueryRowContext(ctx, `
SELECT project_id, status, decision_kind
FROM decision_ledgers_v23
WHERE id = $1
`, decisionID).Scan(&curProjectID, &curStatus, &curKind)
	log.Printf("decision_row_before: id=%d project_id=%s status=%s kind=%s", decisionID, curProjectID, curStatus, curKind)

	var retID sql.NullInt64
	var retStatus sql.NullString
	err = db.QueryRowContext(ctx, `SELECT * FROM decision_approve_v23($1,$2,$3,$4,$5)`,
		decisionID, projectID, "human", actorID, input64(0),
	).Scan(&retID, &retStatus)
	if err != nil {
		log.Printf("decision_approve_v23 Scan error: %v (will verify DB state)", err)
	}

	var afterStatus, afterKind string
	err2 := db.QueryRowContext(ctx, `
SELECT status, decision_kind
FROM decision_ledgers_v23
WHERE id=$1
`, decisionID).Scan(&afterStatus, &afterKind)
	if err2 != nil {
		log.Fatalf("verify decision status failed: %v", err2)
	}
	log.Printf("decision_row_after: id=%d status=%s kind=%s (scan_returned id=%v status=%v)",
		decisionID, afterStatus, afterKind, retID, retStatus)
	if afterStatus != "approved" {
		log.Fatalf("approve did not result in approved status (status=%s).", afterStatus)
	}
	log.Printf("decision_approved: id=%d status=%s", decisionID, afterStatus)

	// ---------------------------------------------------------------------
	// 4) decision_action_enqueue_v23
	// ---------------------------------------------------------------------
	actionType := "publish_http"
	actionScope := "managed"
	targetHash := sha256hex(fmt.Sprintf("target:%d", targetAssetID))
	actionKey := sha256hex(fmt.Sprintf("%s|%d|%s|%s|%s|%s", projectID, decisionID, actionType, actionScope, targetHash, policyVersionStr))

	var actionID int64
	err = db.QueryRowContext(ctx, `
SELECT decision_action_enqueue_v23(
  $1,$2,$3::uuid,$4,
  $5,$6,$7,
  $8,$9,$10,
  $11,$12,
  $13,$14
)`,
		projectID,
		traceID,
		runID,
		decisionID,
		actionKey,
		actionType,
		actionScope,
		targetHash,
		input64(targetAssetID),
		input64(planAssetID),
		"usd_micros",
		int64(1000),
		"queued",
		input64(0),
	).Scan(&actionID)
	if err != nil {
		log.Fatalf("decision_action_enqueue_v23 failed: %v", err)
	}
	log.Printf("action_enqueued: id=%d action_key=%s", actionID, actionKey)

	// ---------------------------------------------------------------------
	// 5) decision_action_claim_next_v23 + mark_succeeded
	// ---------------------------------------------------------------------
	var claimed struct {
		ActionID  int64
		ActionKey string
		Type      string
		Scope     string
		Decision  int64
		TraceID   string
		RunID     string
	}
	row := db.QueryRowContext(ctx, `
SELECT action_id, action_key, action_type, action_scope, decision_ledger_id, trace_id, run_id::text
FROM decision_action_claim_next_v23($1)
`, projectID)

	err = row.Scan(&claimed.ActionID, &claimed.ActionKey, &claimed.Type, &claimed.Scope, &claimed.Decision, &claimed.TraceID, &claimed.RunID)
	if err != nil {
		log.Fatalf("decision_action_claim_next_v23 failed: %v", err)
	}
	log.Printf("action_claimed: id=%d key=%s type=%s scope=%s decision=%d trace=%s run=%s",
		claimed.ActionID, claimed.ActionKey, claimed.Type, claimed.Scope, claimed.Decision, claimed.TraceID, claimed.RunID)

	_, err = db.ExecContext(ctx, `SELECT decision_action_mark_succeeded_v23($1,$2)`, claimed.ActionID, projectID)
	if err != nil {
		log.Fatalf("decision_action_mark_succeeded_v23 failed: %v", err)
	}
	log.Printf("action_mark_succeeded: id=%d", claimed.ActionID)

	log.Printf("OK v23 decisioncore smoke (exec-only v18 evidence): project_id=%s run=%s trace=%s decision=%d action=%d",
		projectID, runID, traceID, decisionID, actionID)
}

// ---------------- helpers ----------------

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}
func envString(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func mustCreateRun(ctx context.Context, db *sql.DB, projectID, pipelineVersion string) string {
	var runID string
	err := db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, pipeline_version)
VALUES ($1, $2)
RETURNING run_id::text
`, projectID, pipelineVersion).Scan(&runID)
	if err != nil {
		log.Fatalf("create run failed: %v", err)
	}
	return runID
}

func input64(v int64) int64 { return v }

// projects.id is string (e.g. akproj_...).
func mustPickProjectsID(ctx context.Context, dsn string, preferred string) string {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error in mustPickProjectsID: %v", err)
	}
	defer db.Close()

	if strings.TrimSpace(preferred) != "" {
		var one int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM public.projects WHERE id=$1 LIMIT 1`, preferred).Scan(&one); err == nil {
			return preferred
		}
		log.Printf("AK_PROJECT_ID=%q not found in public.projects.id; will pick one automatically", preferred)
	}

	var id string
	err = db.QueryRowContext(ctx, `SELECT id FROM public.projects ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err != nil {
		log.Fatalf("failed to pick projects.id: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		log.Fatalf("picked empty projects.id")
	}
	return id
}

func mustRegisterEvidenceAssetIDViaV18(
	ctx context.Context,
	bridge *postgres.EvidenceV18Bridge,
	projectID, traceID, actorType, actorID, kind, payloadJSON, idemKey string,
) int64 {
	sha := sha256hex(strings.ToLower(strings.TrimSpace(payloadJSON)))
	if len(sha) != 64 {
		log.Fatalf("sha256 length invalid: %s", sha)
	}

	// v18 requires lowercase hex sha; our sha256hex already produces lowercase.
	res, err := bridge.Register(ctx, postgres.EvidenceRegisterV18Input{
		ProjectID:       projectID,
		TraceID:         traceID,
		ActorType:       actorType,
		ActorID:         actorID,
		MediaType:       "text",
		MimeType:        "application/json",
		SourceKind:      "generated",
		SourceURI:       fmt.Sprintf("smoke://%s/%d", kind, time.Now().UnixNano()),
		ContentSHA256:   sha,
		ContentLength:   int64(len(payloadJSON)),
		Language:        "en",
		RetentionPolicy: "standard",
		ExpiresAtUTC:    nil,
		IdempotencyKey:  idemKey,
	})
	if err != nil {
		log.Fatalf("evidence_register_v18 failed: %v", err)
	}
	return res.EvidenceAssetID
}