package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type runRow struct {
	RunID   string
	TraceID string
}

func main() {
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" {
		log.Fatal("AK_DB_DSN is required")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	redisAddr := os.Getenv("AK_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "ak_redis:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	poll := 500 * time.Millisecond
	if v := os.Getenv("AK_WORKER_POLL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			poll = time.Duration(n) * time.Millisecond
		}
	}

	// v3.1 stub cost
	cost := int64(10)
	if v := os.Getenv("AK_RUN_COST"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cost = n
		}
	}

	log.Printf("AK Go Worker started. redis=%s poll=%s cost=%d", redisAddr, poll, cost)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		run, ok := pickQueuedRun(ctx, db)
		cancel()

		if !ok {
			time.Sleep(poll)
			continue
		}

		// Redis lock: prevent double-processing (best-effort; DB is final guard)
		lockKey := "run:" + run.RunID + ":lock"
		acqCtx, acqCancel := context.WithTimeout(context.Background(), 2*time.Second)
		locked, err := rdb.SetNX(acqCtx, lockKey, "1", 60*time.Second).Result()
		acqCancel()
		if err != nil {
			log.Printf("redis error: %v", err)
			continue
		}
		if !locked {
			_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.skipped.already_running", "queued", map[string]any{
				"lock": "redis_setnx_failed",
			})
			continue
		}

		func() {
			defer func() {
				relCtx, relCancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, _ = rdb.Del(relCtx, lockKey).Result()
				relCancel()
			}()

			projectID, err := getRunProjectID(context.Background(), db, run.RunID)
			if err != nil {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.failed", "failed", map[string]any{
					"error": "project_id_lookup_failed",
				})
				return
			}

			// ---- v3.1 budget gate + spend (atomic) ----
			blockedEvent, blockedPayload, err := gateAndSpendBudgetTx(
				context.Background(), db,
				run.RunID, run.TraceID, projectID,
				cost,
			)
			if err != nil {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.failed", "failed", map[string]any{
					"error": "budget_gate_tx_failed",
				})
				return
			}
			if blockedEvent != "" {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, blockedEvent, "failed", blockedPayload)
				return
			}

			// ---- proceed ----
			if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.running", "running", nil); err != nil {
				log.Printf("run.running append failed: %v", err)
				return
			}

			// stub: pretend work
			time.Sleep(200 * time.Millisecond)

			if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.succeeded", "done", map[string]any{
				"result": "stub_ok",
			}); err != nil {
				log.Printf("run.succeeded append failed: %v", err)
				return
			}
		}()
	}
}

// pick the oldest queued run safely (SKIP LOCKED)
func pickQueuedRun(ctx context.Context, db *pgxpool.Pool) (runRow, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return runRow{}, false
	}
	defer tx.Rollback(ctx)

	var r runRow
	err = tx.QueryRow(ctx, `
		SELECT run_id, trace_id
		FROM runs
		WHERE status = 'queued'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&r.RunID, &r.TraceID)

	if err != nil {
		return runRow{}, false
	}

	_, _ = tx.Exec(ctx, `UPDATE runs SET updated_at = now() WHERE run_id = $1`, r.RunID)

	if err := tx.Commit(ctx); err != nil {
		return runRow{}, false
	}
	return r, true
}

// append event with next seq + update status
func appendEventAndStatus(ctx context.Context, db *pgxpool.Pool, runID, traceID, eventName, newStatus string, payload map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var nextSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0) + 1
		FROM run_events
		WHERE run_id = $1
	`, runID).Scan(&nextSeq)
	if err != nil {
		return err
	}

	var pb []byte
	if payload == nil {
		pb = []byte(`{}`)
	} else {
		pb, _ = json.Marshal(payload)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, nextSeq, eventName, string(pb))
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET status = $2, updated_at = now()
		WHERE run_id = $1
	`, runID, newStatus)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func getRunProjectID(ctx context.Context, db *pgxpool.Pool, runID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var projectID string
	err := db.QueryRow(ctx, `
		SELECT project_id
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(&projectID)
	return projectID, err
}

// gateAndSpendBudgetTx:
// - Locks project_budgets row (FOR UPDATE) to make gate+spend atomic per project
// - Enforces per_run_limit (per run sum) and daily_limit (project sum today)
// - Inserts budget_ledger (append-only). 23505 (ux_budget_ledger_run_reason) is treated as success.
func gateAndSpendBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	cost int64,
) (blockedEvent string, blockedPayload map[string]any, err error) {

	if cost <= 0 {
		return "", nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	// 1) Lock budget row (serializes per-project budget decisions)
	var perRunLimit int64
	var dailyLimit int64
	err = tx.QueryRow(ctx, `
		SELECT per_run_limit, daily_limit
		FROM project_budgets
		WHERE project_id = $1
		FOR UPDATE
	`, projectID).Scan(&perRunLimit, &dailyLimit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// safest: if budget row missing => block
			return "run.blocked.budget", map[string]any{
				"project_id":    projectID,
				"per_run_limit": int64(0),
				"daily_limit":   int64(0),
				"needed":        cost,
				"unit":          "credits",
				"reason":        "budget_row_missing",
			}, nil
		}
		return "", nil, err
	}

	// 2) per-run guard (sum by run_id)
	var spentRun int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM budget_ledger
		WHERE run_id = $1
	`, runID).Scan(&spentRun)
	if err != nil {
		return "", nil, err
	}
	if perRunLimit <= 0 {
		return "run.blocked.budget", map[string]any{
			"project_id":    projectID,
			"per_run_limit": perRunLimit,
			"spent":         spentRun,
			"needed":        cost,
			"unit":          "credits",
			"reason":        "no_per_run_budget",
		}, nil
	}
	if spentRun+cost > perRunLimit {
		return "run.blocked.budget", map[string]any{
			"project_id":    projectID,
			"per_run_limit": perRunLimit,
			"spent":         spentRun,
			"needed":        cost,
			"unit":          "credits",
			"reason":        "per_run_limit_exceeded",
		}, nil
	}

	// 3) daily guard (sum by project_id today)
	if dailyLimit > 0 {
		var spentToday int64
		err = tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0)
			FROM budget_ledger
			WHERE project_id = $1
			  AND created_at >= date_trunc('day', now())
		`, projectID).Scan(&spentToday)
		if err != nil {
			return "", nil, err
		}
		if spentToday+cost > dailyLimit {
			return "run.blocked.budget.daily", map[string]any{
				"project_id":  projectID,
				"daily_limit": dailyLimit,
				"spent_today": spentToday,
				"needed":      cost,
				"unit":        "credits",
				"reason":      "daily_limit_exceeded",
			}, nil
		}
	}

	// 4) spend (append-only)
	//    unique(run_id, reason) makes it idempotent
	_, err = tx.Exec(ctx, `
		INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, runID, traceID, projectID, cost, "credits", "stub_run_cost")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// already spent (idempotent) => treat as success
		} else {
			return "", nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}