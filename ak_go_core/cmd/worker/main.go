package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type runRow struct {
	RunID  string
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
		// optional: keep simple; ignore parse errors
	}

	log.Printf("AK Go Worker started. redis=%s", redisAddr)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		run, ok := pickQueuedRun(ctx, db)
		cancel()

		if !ok {
			time.Sleep(poll)
			continue
		}

		// Redis lock: prevent double-processing
		lockKey := "run:" + run.RunID + ":lock"
		acqCtx, acqCancel := context.WithTimeout(context.Background(), 2*time.Second)
		locked, err := rdb.SetNX(acqCtx, lockKey, "1", 60*time.Second).Result()
		acqCancel()
		if err != nil {
			log.Printf("redis error: %v", err)
			continue
		}
		if !locked {
			// already running elsewhere: record and continue
			_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.skipped.already_running", "queued", nil)
			continue
		}

		// Process run (stub): running -> succeeded
		if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.running", "running", nil); err != nil {
			log.Printf("run.running append failed: %v", err)
			continue
		}

		// TODO: call real pipeline / tools
		time.Sleep(200 * time.Millisecond)

		if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID, "run.succeeded", "done", map[string]any{
			"result": "stub_ok",
		}); err != nil {
			log.Printf("run.succeeded append failed: %v", err)
			continue
		}

		// release lock (best-effort)
		relCtx, relCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = rdb.Del(relCtx, lockKey).Result()
		relCancel()
	}
}

// pick the oldest queued run (naive v3 stub)
func pickQueuedRun(ctx context.Context, db *pgxpool.Pool) (runRow, bool) {
	var r runRow
	err := db.QueryRow(ctx, `
		SELECT run_id, trace_id
		FROM runs
		WHERE status = 'queued'
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&r.RunID, &r.TraceID)
	if err != nil {
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
