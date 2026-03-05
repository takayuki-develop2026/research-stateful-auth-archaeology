package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/ak_go_core/internal/worker"
)

func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func main() {
	// ---- config
	dsn := envOr("AK_DB_DSN", envOr("DATABASE_URL", ""))
	if dsn == "" {
		log.Fatal("missing AK_DB_DSN (or DATABASE_URL)")
	}
	workerID := envOr("AK_WORKER_ID", envOr("AK_WORKER_NAME", "worker-1"))

	poll := envOr("AK_WORKER_POLL", "500ms")
	pollDur, err := time.ParseDuration(poll)
	if err != nil || pollDur <= 0 {
		pollDur = 500 * time.Millisecond
	}

	// ---- ctx / shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigC := make(chan os.Signal, 2)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigC
		cancel()
	}()

	// ---- db
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer pool.Close()

	st := worker.NewStore(pool)

	log.Printf("[worker] start id=%s poll=%s", workerID, pollDur)

	t := time.NewTicker(pollDur)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[worker] stop: %v", ctx.Err())
			return

		case <-t.C:
			r, ok, err := st.PickQueuedRun(ctx, workerID)
			if err != nil {
				log.Printf("[worker] PickQueuedRun error: %v", err)
				continue
			}
			if !ok {
				continue
			}

			// Placeholder work (keep tiny; real work is in pisag_go worker later)
			time.Sleep(50 * time.Millisecond)

			// IMPORTANT:
			// run_events has no event_seq in your DB, so AppendEvent* cannot be used here.
			// We finalize via runs.status only.
			if err := st.MarkDone(ctx, r.RunID); err != nil {
				log.Printf("[worker] MarkDone error run_id=%s: %v", r.RunID, err)

				// best-effort: mark failed (if implemented)
				_ = st.MarkFailed(ctx, r.RunID, "worker_finish_failed", err.Error())
				continue
			}

			log.Printf("[worker] done run_id=%s trace_id=%s", r.RunID, r.TraceID)
		}
	}
}