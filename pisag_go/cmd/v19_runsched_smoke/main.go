package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic("missing env: " + k)
	}
	return v
}
func envOr(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func main() {
	ctx := context.Background()
	dsn := mustEnv("AK_PG_DSN")
	projectID := envOr("AK_PROJECT_ID", "demo")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	now := time.Now().UTC()

	// 0) sanity
	var one int
	if err := db.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		panic(err)
	}

	// 1) evidence assets (budget/policy/input/reason)
	budgetEvID := mustInsertEvidenceAsset(ctx, db, "v19_smoke_budget_policy")
	retryEvID := mustInsertEvidenceAsset(ctx, db, "v19_smoke_retry_policy")
	inputEvID := mustInsertEvidenceAsset(ctx, db, "v19_smoke_input_template")

	reasonBudgetEv := mustInsertEvidenceAsset(ctx, db, "v19_smoke_reason_budget_deny")
	reasonPolicyEv := mustInsertEvidenceAsset(ctx, db, "v19_smoke_reason_policy_deny")

	// 2) ensure task_type_contract exists (v18)
	taskType := "demo_task"
	pipelineVersion := "v1"
	{
		_, err := db.Exec(ctx, `
			INSERT INTO task_type_contracts(project_id, task_type, pipeline_version, enabled, created_at)
			VALUES ($1,$2,$3,TRUE,now())
			ON CONFLICT (project_id, task_type, pipeline_version) DO UPDATE SET enabled=EXCLUDED.enabled
		`, projectID, taskType, pipelineVersion)
		if err != nil {
			panic(fmt.Errorf("insert task_type_contracts failed: %w", err))
		}
	}

	// 3) insert 3 schedules due now-1s
	nextRun := now.Add(-1 * time.Second)
	policyVersionID := "pol_v1_published"

	schedBudgetDeny := mustInsertSchedule(ctx, db, projectID, "sched_budget_deny", taskType, pipelineVersion, policyVersionID, inputEvID, budgetEvID, retryEvID, nextRun)
	schedPolicyDeny := mustInsertSchedule(ctx, db, projectID, "sched_policy_deny", taskType, pipelineVersion, policyVersionID, inputEvID, budgetEvID, retryEvID, nextRun)
	schedAllow := mustInsertSchedule(ctx, db, projectID, "sched_allow", taskType, pipelineVersion, policyVersionID, inputEvID, budgetEvID, retryEvID, nextRun)

	// 4) tick enqueue (limit sufficiently large)
	{
		_, err := db.Exec(ctx, `
			SELECT count(*) FROM runsched_tick_enqueue_v19($1,$2,$3,'{}'::jsonb)
		`, projectID, now, 50)
		if err != nil {
			panic(fmt.Errorf("runsched_tick_enqueue_v19 failed: %w", err))
		}
	}

	// 5) claim queued scheduled_runs
	type Claimed struct {
		ScheduledRunID int64
		ScheduleID     int64
		TraceID        string
	}
	claimed := make([]Claimed, 0, 16)
	{
		rows, err := db.Query(ctx, `
			SELECT scheduled_run_id, schedule_id, trace_id
			FROM runsched_claim_queued_scheduled_runs_v19($1,$2)
		`, projectID, 50)
		if err != nil {
			panic(fmt.Errorf("runsched_claim_queued_scheduled_runs_v19 failed: %w", err))
		}
		defer rows.Close()
		for rows.Next() {
			var c Claimed
			if err := rows.Scan(&c.ScheduledRunID, &c.ScheduleID, &c.TraceID); err != nil {
				panic(err)
			}
			claimed = append(claimed, c)
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}
	}

	if len(claimed) == 0 {
		panic("no queued scheduled_runs claimed")
	}

	// 6) apply gates:
	// - sched_budget_deny: mark skipped_budget
	// - sched_policy_deny: mark skipped_policy
	// - sched_allow: create run
	var skippedBudgetID int64
	var skippedPolicyID int64
	var dispatchedID int64
	var runID string

	for _, c := range claimed {
		switch c.ScheduleID {
		case schedBudgetDeny:
			// budget gate denies
			if err := markSkipped(ctx, db, projectID, c.ScheduledRunID, "skipped_budget", "budget_cap_exceeded", reasonBudgetEv); err != nil {
				panic(err)
			}
			skippedBudgetID = c.ScheduledRunID

		case schedPolicyDeny:
			// policy gate denies
			if err := markSkipped(ctx, db, projectID, c.ScheduledRunID, "skipped_policy", "policy_default_deny", reasonPolicyEv); err != nil {
				panic(err)
			}
			skippedPolicyID = c.ScheduledRunID

		case schedAllow:
			// both allow -> create run
			sid, rid, st, err := createRun(ctx, db, projectID, c.ScheduledRunID, now)
			if err != nil {
				panic(err)
			}
			if st != "dispatched" || rid == "" {
				panic(fmt.Errorf("expected dispatched, got status=%s run_id=%s", st, rid))
			}
			dispatchedID = sid
			runID = rid
		}
	}

	if skippedBudgetID == 0 || skippedPolicyID == 0 || dispatchedID == 0 || runID == "" {
		panic(fmt.Errorf("missing expected results: skipped_budget=%d skipped_policy=%d dispatched=%d run_id=%s",
			skippedBudgetID, skippedPolicyID, dispatchedID, runID))
	}

	// 7) verify DB states
	// 7.1 skipped_budget
	assertScheduledRun(ctx, db, projectID, skippedBudgetID, "skipped_budget", "")
	// 7.2 skipped_policy
	assertScheduledRun(ctx, db, projectID, skippedPolicyID, "skipped_policy", "")
	// 7.3 dispatched
	assertScheduledRun(ctx, db, projectID, dispatchedID, "dispatched", runID)

	// 7.4 run row exists
	{
		var st, tt, pid string
		err := db.QueryRow(ctx, `SELECT status, task_type, project_id FROM runs WHERE id=$1`, runID).Scan(&st, &tt, &pid)
		if err != nil {
			panic(err)
		}
		if st != "queued" {
			panic("run status should be queued: " + st)
		}
		if pid != projectID || tt != taskType {
			panic(fmt.Errorf("run mismatch: project=%s task=%s", pid, tt))
		}
	}

	fmt.Println("[v19_runsched_smoke] OK")
	fmt.Println(" project_id:", projectID)
	fmt.Println(" schedule_ids:", schedBudgetDeny, schedPolicyDeny, schedAllow)
	fmt.Println(" scheduled_run_ids:", skippedBudgetID, skippedPolicyID, dispatchedID)
	fmt.Println(" run_id:", runID)
	fmt.Println(" check:", sha(projectID+"|"+runID))
}

func mustInsertSchedule(ctx context.Context, db *pgxpool.Pool, projectID, name, taskType, pipelineVersion, policyVersionID string,
	inputEv, budgetEv, retryEv int64, nextRun time.Time,
) int64 {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO run_schedules(
			project_id,name,enabled,
			schedule_kind,interval_seconds,timezone,
			task_type,pipeline_version,policy_version_id,mode,
			priority,next_run_at_utc,
			input_template_evidence_asset_id,budget_policy_evidence_asset_id,retry_policy_evidence_asset_id,
			concurrency_policy,max_concurrent_runs,
			created_by_type,created_by_id
		)
		VALUES (
			$1,$2,TRUE,
			'interval',$3,'UTC',
			$4,$5,$6,NULL,
			50,$7,
			$8,$9,$10,
			'singleton',1,
			'system',NULL
		)
		RETURNING id
	`, projectID, name, 60, taskType, pipelineVersion, policyVersionID, nextRun, inputEv, budgetEv, retryEv).Scan(&id)
	if err != nil {
		panic(fmt.Errorf("insert run_schedules failed (%s): %w", name, err))
	}
	return id
}

func markSkipped(ctx context.Context, db *pgxpool.Pool, projectID string, scheduledRunID int64, status, reasonCode string, reasonEv int64) error {
	_, err := db.Exec(ctx, `SELECT runsched_mark_skipped_v19($1,$2,$3,$4,$5)`, projectID, scheduledRunID, status, reasonCode, reasonEv)
	if err != nil {
		return fmt.Errorf("runsched_mark_skipped_v19 failed: %w", err)
	}
	return nil
}

func createRun(ctx context.Context, db *pgxpool.Pool, projectID string, scheduledRunID int64, now time.Time) (int64, string, string, error) {
	var sid int64
	var runID string
	var traceID string
	var status string
	err := db.QueryRow(ctx, `
		SELECT scheduled_run_id, run_id, trace_id, status
		FROM runsched_create_run_for_scheduled_v19($1,$2,$3)
	`, projectID, scheduledRunID, now).Scan(&sid, &runID, &traceID, &status)
	if err != nil {
		return 0, "", "", fmt.Errorf("runsched_create_run_for_scheduled_v19 failed: %w", err)
	}
	_ = traceID
	return sid, runID, status, nil
}

func assertScheduledRun(ctx context.Context, db *pgxpool.Pool, projectID string, scheduledRunID int64, wantStatus, wantRunID string) {
	var st string
	var rid *string
	err := db.QueryRow(ctx, `
		SELECT status, run_id
		FROM scheduled_runs
		WHERE id=$1 AND project_id=$2
	`, scheduledRunID, projectID).Scan(&st, &rid)
	if err != nil {
		panic(err)
	}
	if st != wantStatus {
		panic(fmt.Errorf("scheduled_runs status mismatch: got=%s want=%s (id=%d)", st, wantStatus, scheduledRunID))
	}
	if wantRunID == "" {
		if rid != nil && *rid != "" {
			panic(fmt.Errorf("scheduled_runs run_id should be empty, got=%s", *rid))
		}
		return
	}
	if rid == nil || *rid != wantRunID {
		got := "<nil>"
		if rid != nil {
			got = *rid
		}
		panic(fmt.Errorf("scheduled_runs run_id mismatch: got=%s want=%s", got, wantRunID))
	}
}

//
// Evidence insertion that adapts to your v18 evidence_assets constraints
//
func mustInsertEvidenceAsset(ctx context.Context, db *pgxpool.Pool, label string) int64 {
	// First try: DEFAULT VALUES
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO evidence_assets DEFAULT VALUES RETURNING id`).Scan(&id)
	if err == nil {
		return id
	}

	// If it fails, auto-build INSERT for NOT NULL columns without default.
	cols, err2 := requiredColumnsNoDefault(ctx, db, "public", "evidence_assets")
	if err2 != nil {
		panic(fmt.Errorf("evidence_assets insert failed and cannot inspect schema: %v / %v", err, err2))
	}
	if len(cols) == 0 {
		// try again with minimal explicit created_at if exists
		err3 := db.QueryRow(ctx, `INSERT INTO evidence_assets(created_at) VALUES (now()) RETURNING id`).Scan(&id)
		if err3 == nil {
			return id
		}
		panic(fmt.Errorf("evidence_assets insert failed (fallback): %v / %v", err, err3))
	}

	colNames := make([]string, 0, len(cols))
	argExprs := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))

	for i, c := range cols {
		colNames = append(colNames, c.Name)
		argExprs = append(argExprs, fmt.Sprintf("$%d", i+1))
		args = append(args, dummyValueForType(label, c.UDTName))
	}

	sql := fmt.Sprintf(
		`INSERT INTO evidence_assets(%s) VALUES (%s) RETURNING id`,
		strings.Join(colNames, ","),
		strings.Join(argExprs, ","),
	)

	// If there is a sha256-like column, set deterministic value (helps uniqueness).
	// We already set dummy values; just attempt.
	err = db.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		// As a last resort, show the original error
		var pgErr *pgconn.PgError
		if ok := errorAs(err, &pgErr); ok {
			panic(fmt.Errorf("evidence_assets dynamic insert failed: %s (%s) detail=%s hint=%s",
				pgErr.Message, pgErr.Code, pgErr.Detail, pgErr.Hint))
		}
		panic(fmt.Errorf("evidence_assets dynamic insert failed: %w (initial: %v)", err, err))
	}
	return id
}

// small helper: avoid importing errors package name clash in old environments
func errorAs(err error, target any) bool {
	switch t := target.(type) {
	case **pgconn.PgError:
		if e, ok := err.(*pgconn.PgError); ok {
			*t = e
			return true
		}
		return false
	default:
		return false
	}
}

type reqCol struct {
	Name    string
	UDTName string
}

func requiredColumnsNoDefault(ctx context.Context, db *pgxpool.Pool, schema, table string) ([]reqCol, error) {
	// NOT NULL and no default, excluding generated identity/serial columns
	rows, err := db.Query(ctx, `
		SELECT a.attname,
		       t.typname
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
		WHERE n.nspname=$1
		  AND c.relname=$2
		  AND a.attnum > 0
		  AND a.attisdropped = FALSE
		  AND a.attnotnull = TRUE
		  AND d.oid IS NULL
		  AND a.attgenerated = ''
		  AND a.attidentity = ''
		ORDER BY a.attnum
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []reqCol
	for rows.Next() {
		var n, typ string
		if err := rows.Scan(&n, &typ); err != nil {
			return nil, err
		}
		// if id is required but has identity in your schema, it won't appear here
		out = append(out, reqCol{Name: n, UDTName: typ})
	}
	return out, rows.Err()
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func dummyValueForType(label string, typ string) any {
	switch typ {
	case "text":
		return label
	case "varchar", "bpchar":
		// IMPORTANT: avoid common varchar(16) constraints in your schema
		return clip(label, 16)
	case "bool":
		return false
	case "int2", "int4", "int8":
		return int64(0)
	case "numeric":
		return "0"
	case "timestamptz", "timestamp":
		return time.Now().UTC()
	case "date":
		return time.Now().UTC().Format("2006-01-02")
	case "jsonb":
		return "{}"
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	case "bytea":
		return []byte{}
	default:
		// safest: short string
		return clip(label, 16)
	}
}