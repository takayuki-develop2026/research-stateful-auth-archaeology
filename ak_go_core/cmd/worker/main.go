package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
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
	dsn := mustEnv("AK_DB_DSN")
	workerID := getenvDefault("AK_WORKER_ID", hostnameFallback())

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	redisAddr := getenvDefault("AK_REDIS_ADDR", "ak_redis:6379")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	poll := getenvDurationMS("AK_WORKER_POLL_MS", 500*time.Millisecond)
	cost := getenvInt64("AK_RUN_COST", 10)

	log.Printf("AK Go Worker started. worker_id=%s redis=%s poll=%s cost=%d", workerID, redisAddr, poll, cost)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		run, ok, pickErr := pickQueuedRun(ctx, db)
		cancel()

		if pickErr != nil {
			log.Printf("pickQueuedRun error: %v", pickErr)
			time.Sleep(poll)
			continue
		}
		if !ok {
			time.Sleep(poll)
			continue
		}

		// Redis lock: prevent double-processing (best-effort; DB is final guard)
		lockKey := "run:" + run.RunID + ":lock"
		locked, err := redisSetNX(context.Background(), rdb, lockKey, "1", 60*time.Second)
		if err != nil {
			log.Printf("redis SetNX error: %v", err)
			continue
		}
		if !locked {
			// another worker already owns lock
			_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
				"run.skipped.already_running", "queued",
				map[string]any{"lock": "already_held"},
			)
			continue
		}

		func() {
			defer func() { _ = redisDel(context.Background(), rdb, lockKey) }()

			projectID, err := getRunProjectID(context.Background(), db, run.RunID)
			if err != nil {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					"run.failed", "failed",
					map[string]any{
						"error_code":   "project_id_lookup_failed",
						"error_detail": err.Error(),
						"worker_id":    workerID,
					},
				)
				return
			}

			pickedAt := time.Now().UTC().Format(time.RFC3339Nano)

			// mark running
			if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
				"run.running", "running",
				map[string]any{
					"worker_id": workerID,
					"picked_at": pickedAt,
				},
			); err != nil {
				log.Printf("run.running append failed: %v", err)
				return
			}

			// ---- v3.1 budget gate + spend (atomic) ----
			blockedEvent, blockedPayload, err := gateAndSpendBudgetTx(
				context.Background(), db,
				run.RunID, run.TraceID, projectID,
				cost,
			)
			if err != nil {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					"run.failed", "failed",
					map[string]any{
						"error_code":   "budget_gate_tx_failed",
						"error_detail": err.Error(),
						"worker_id":    workerID,
					},
				)
				return
			}
			if blockedEvent != "" {
				// budget block => failed in v3.1 (future: review_required)
				if blockedPayload == nil {
					blockedPayload = map[string]any{}
				}
				blockedPayload["worker_id"] = workerID
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					blockedEvent, "failed", blockedPayload,
				)
				return
			}

			// ---- v3.2: mode from run.enqueued payload ----
			mode, modeErr := getRunModeFromEnqueuedEvent(context.Background(), db, run.RunID)
			if modeErr != nil {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					"run.failed", "failed",
					map[string]any{
						"error_code":   "mode_lookup_failed",
						"error_detail": modeErr.Error(),
						"worker_id":    workerID,
					},
				)
				return
			}

			// ---- v3.2: Router (stub) ----
			routeDecision := decideRoute(mode)

			// learn_signals: mode_received
			_ = insertLearnSignal(context.Background(), db, run.RunID, projectID,
				"mode_received",
				map[string]any{"mode": mode, "worker_id": workerID},
			)

			// artifact: route_decision
			_ = upsertRunArtifact(context.Background(), db, run.RunID, "route_decision", map[string]any{
				"mode":      mode,
				"route_id":  routeDecision.RouteID,
				"provider":  routeDecision.Provider,
				"model":     routeDecision.Model,
				"decidedAt": time.Now().UTC().Format(time.RFC3339Nano),
			})

			// pretend work
			time.Sleep(200 * time.Millisecond)

			// artifact: analysis_result
			_ = upsertRunArtifact(context.Background(), db, run.RunID, "analysis_result", map[string]any{
				"ok":   true,
				"mode": mode,
				"stub": "result_v3_2",
			})

			// learn_signals: route_selected
			_ = insertLearnSignal(context.Background(), db, run.RunID, projectID,
				"route_selected",
				map[string]any{
					"mode":     mode,
					"route_id": routeDecision.RouteID,
					"route":    "stub",
					"provider": routeDecision.Provider,
					"model":    routeDecision.Model,
				},
			)

			// success
			if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
				"run.succeeded", "done",
				map[string]any{
					"result":    "stub_ok",
					"mode":      mode,
					"route_id":  routeDecision.RouteID,
					"provider":  routeDecision.Provider,
					"model":     routeDecision.Model,
					"worker_id": workerID,
				},
			); err != nil {
				log.Printf("run.succeeded append failed: %v", err)
				return
			}
		}()
	}
}

type RouteDecision struct {
	RouteID  string
	Provider string
	Model    string
}

func decideRoute(mode int) RouteDecision {
	// v3.2 minimal routing table (stub)
	switch mode {
	case 1:
		return RouteDecision{RouteID: "r1", Provider: "stub", Model: "stub"}
	default:
		return RouteDecision{RouteID: "r0", Provider: "stub", Model: "stub"}
	}
}

// pick the oldest queued run safely (SKIP LOCKED)
func pickQueuedRun(ctx context.Context, db *pgxpool.Pool) (runRow, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return runRow{}, false, err
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
		if errors.Is(err, pgx.ErrNoRows) {
			return runRow{}, false, nil
		}
		return runRow{}, false, err
	}

	// touch updated_at (optional)
	_, _ = tx.Exec(ctx, `UPDATE runs SET updated_at = now() WHERE run_id = $1`, r.RunID)

	if err := tx.Commit(ctx); err != nil {
		return runRow{}, false, err
	}
	return r, true, nil
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

	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, nextSeq, eventName, pb)
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
// - Inserts budget_ledger (append-only).
// - 23505 is treated as success ONLY when the unique constraint is ux_budget_ledger_run_reason.
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

	// 1) Lock budget row
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

	// 2) per-run guard
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

	// 3) daily guard
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

	// 4) spend (idempotent on (run_id, reason))
	_, err = tx.Exec(ctx, `
		INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, runID, traceID, projectID, cost, "credits", "stub_run_cost")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// allow ONLY the expected unique constraint
			if pgErr.ConstraintName == "ux_budget_ledger_run_reason" {
				// already spent => success
			} else {
				return "", nil, err
			}
		} else {
			return "", nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func getRunModeFromEnqueuedEvent(ctx context.Context, db *pgxpool.Pool, runID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var modeText string
	err := db.QueryRow(ctx, `
		SELECT COALESCE(payload->>'mode', '')
		FROM run_events
		WHERE run_id = $1 AND event_name = 'run.enqueued'
		ORDER BY event_seq ASC
		LIMIT 1
	`, runID).Scan(&modeText)
	if err != nil {
		return 0, err
	}

	modeText = strings.TrimSpace(modeText)
	if modeText == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(modeText)
	if err != nil {
		return 0, nil // safest: invalid -> 0
	}
	return n, nil
}

func upsertRunArtifact(ctx context.Context, db *pgxpool.Pool, runID, kind string, content any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	b, _ := json.Marshal(content)

	_, err := db.Exec(ctx, `
		INSERT INTO run_artifacts(run_id, artifact_kind, content_json, created_at, updated_at)
		VALUES ($1,$2,$3::jsonb, now(), now())
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET content_json = EXCLUDED.content_json, updated_at = now()
	`, runID, kind, string(b))
	return err
}

func insertLearnSignal(ctx context.Context, db *pgxpool.Pool, runID, projectID, signalType string, payload map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pb := marshalJSONOrEmpty(payload)

	_, err := db.Exec(ctx, `
		INSERT INTO learn_signals(run_id, project_id, signal_type, payload)
		VALUES ($1,$2,$3,$4::jsonb)
	`, runID, projectID, signalType, pb)
	return err
}

// -------- helpers --------

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}

func getenvDefault(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func getenvDurationMS(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return time.Duration(n) * time.Millisecond
}

func getenvInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func hostnameFallback() string {
	h, _ := os.Hostname()
	if strings.TrimSpace(h) == "" {
		return "worker-unknown"
	}
	return h
}

func marshalJSONOrEmpty(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func redisSetNX(ctx context.Context, rdb *redis.Client, key, val string, ttl time.Duration) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return rdb.SetNX(cctx, key, val, ttl).Result()
}

func redisDel(ctx context.Context, rdb *redis.Client, key string) error {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := rdb.Del(cctx, key).Result()
	return err
}