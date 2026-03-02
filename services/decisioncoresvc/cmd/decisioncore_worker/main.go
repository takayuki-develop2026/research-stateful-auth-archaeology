package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := mustEnv("AK_DB_DSN")
	projectID := mustEnv("AK_PROJECT_ID") // v23 claim needs TEXT project_id

	pollMs := envInt("DECISIONCORE_WORKER_POLL_MS", 500)
	workerName := envString("AK_WORKER_NAME", "decisioncore-worker")

	ctx := context.Background()

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

	log.Printf("[%s] start project_id=%s poll_ms=%d", workerName, projectID, pollMs)

	for {
		claimed, err := claimNext(ctx, db, projectID)
		if err != nil {
			log.Printf("[%s] claim error: %v", workerName, err)
			time.Sleep(time.Duration(pollMs) * time.Millisecond)
			continue
		}
		if claimed == nil {
			time.Sleep(time.Duration(pollMs) * time.Millisecond)
			continue
		}

		log.Printf("[%s] claimed action_id=%d type=%s scope=%s decision=%d trace=%s run=%s",
			workerName, claimed.ActionID, claimed.ActionType, claimed.ActionScope, claimed.DecisionLedgerID, claimed.TraceID, claimed.RunID)

		// executor stub: always succeed
		if err := markSucceeded(ctx, db, claimed.ActionID, projectID); err != nil {
			log.Printf("[%s] mark_succeeded error: %v", workerName, err)
		} else {
			log.Printf("[%s] succeeded action_id=%d", workerName, claimed.ActionID)
		}
	}
}

type ClaimedAction struct {
	ActionID        int64
	ActionKey       string
	ActionType      string
	ActionScope     string
	DecisionLedgerID int64
	TraceID         string
	RunID           string
	TargetHash      string
	TargetAssetID   int64
	PlanAssetID     int64
	BudgetCurrency  string
	BudgetEstimate  int64
}

func claimNext(ctx context.Context, db *sql.DB, projectID string) (*ClaimedAction, error) {
	row := db.QueryRowContext(ctx, `
SELECT action_id, action_key, action_type, action_scope, decision_ledger_id, trace_id, run_id::text,
       target_hash, target_evidence_asset_id, plan_evidence_asset_id,
       budget_currency, budget_estimate_amount
FROM decision_action_claim_next_v23($1)
`, projectID)

	var a ClaimedAction
	err := row.Scan(
		&a.ActionID, &a.ActionKey, &a.ActionType, &a.ActionScope, &a.DecisionLedgerID,
		&a.TraceID, &a.RunID,
		&a.TargetHash, &a.TargetAssetID, &a.PlanAssetID,
		&a.BudgetCurrency, &a.BudgetEstimate,
	)
	if err != nil {
		// No queued actions
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func markSucceeded(ctx context.Context, db *sql.DB, actionID int64, projectID string) error {
	_, err := db.ExecContext(ctx, `SELECT decision_action_mark_succeeded_v23($1,$2)`, actionID, projectID)
	return err
}

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

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil || n <= 0 {
		return def
	}
	return n
}