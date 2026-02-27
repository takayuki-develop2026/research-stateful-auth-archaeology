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

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

func main() {
	ctx := context.Background()
	db := mustOpenDB()
	defer db.Close()

	projectID := mustEnv("AK_PROJECT_ID")
	traceID := newTraceIDHex32()
	runID := mustPickOrCreateRun(ctx, db, projectID, traceID, "v8.0")

	// 1) stale対象の candidate を作る（review_required かつ review_requested_at を 8日前に）
	candID := seedStaleCandidate(ctx, db, projectID, runID, traceID)

	// 2) lifecycle job 実行（mark_stale）
	lc := postgres.NewLifecycleRepository(db)
	out, err := lc.RunJob(ctx, run.LifecycleJobRunInput{
		ProjectID: projectID,
		RunID:     runID,
		TraceID:   traceID,
		JobType:   "mark_stale",
		Limit:     50,
		DryRun:    false,
	})
	must(err)
	log.Printf("[v87] job_id=%d scanned=%d changed=%d", out.JobID, out.Scanned, out.Changed)

	// 3) candidate が stale になったか
	var staleAt sql.NullTime
	must(db.QueryRowContext(ctx, `
SELECT stale_at
FROM public.discovery_candidates
WHERE project_id=$1 AND id=$2;
`, projectID, candID).Scan(&staleAt))

	if !staleAt.Valid {
		log.Fatalf("expected stale_at set for candidate_id=%d", candID)
	}

	// 4) lifecycle_events が増えたか（stale_marked）
	var cnt int64
	must(db.QueryRowContext(ctx, `
SELECT count(*)
FROM public.discovery_candidate_lifecycle_events
WHERE project_id=$1 AND candidate_id=$2 AND event_type='stale_marked';
`, projectID, candID).Scan(&cnt))

	if cnt <= 0 {
		log.Fatalf("expected lifecycle_event stale_marked inserted")
	}

	log.Printf("OK: v87 lifecycle smoke passed (candidate=%d)", candID)
}

// ---------- seed helper ----------
func seedStaleCandidate(ctx context.Context, db *sql.DB, projectID, runID, traceID string) int64 {
	// source 1件（FK）
	var sourceID int64
	must(db.QueryRowContext(ctx, `
INSERT INTO public.discovery_sources(
  project_id, source_type, source_ref_raw, source_ref, source_hash,
  run_id, trace_id, pipeline_version, policy_version,
  status, failure_state,
  first_seen_at, last_seen_at, seen_count
)
VALUES (
  $1,'pisag_html','https://example.com/x','https://example.com/x', repeat('a',64),
  $2::uuid,$3,'v8.0','p8.0-default',
  'detected','none',
  now(), now(), 1
)
ON CONFLICT (project_id, source_type, source_hash) DO UPDATE
  SET last_seen_at=now(), seen_count=public.discovery_sources.seen_count+1
RETURNING id;
`, projectID, runID, traceID).Scan(&sourceID))

	var candID int64
	must(db.QueryRowContext(ctx, `
INSERT INTO public.discovery_candidates(
  project_id, source_id,
  candidate_type, candidate_key,
  status, risk_level,
  first_seen_at, last_seen_at, seen_count,
  review_requested_at,
  run_id, trace_id, pipeline_version, policy_version
)
VALUES (
  $1,$2,
  'catalog_source', repeat('b',64),
  'review_required','normal',
  now(), now(), 1,
  now() - interval '8 days',
  $3::uuid,$4,'v8.0','p8.0-default'
)
ON CONFLICT (project_id, candidate_type, candidate_key) DO UPDATE
  SET review_requested_at = now() - interval '8 days',
      status='review_required'
RETURNING id;
`, projectID, sourceID, runID, traceID).Scan(&candID))

	return candID
}

// ---------- shared helpers ----------
func mustOpenDB() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		host := mustEnv("PGHOST")
		port := envOr("PGPORT", "5432")
		dbname := envOr("PGDATABASE", "postgres")
		user := envOr("PGUSER", "postgres")
		pass := os.Getenv("PGPASSWORD")
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
	}
	db, err := sql.Open("pgx", dsn)
	must(err)
	must(db.Ping())
	return db
}

func mustPickOrCreateRun(ctx context.Context, db *sql.DB, projectID, traceID, pipelineVersion string) string {
	var runID string
	err := db.QueryRowContext(ctx, `
SELECT run_id::text
FROM public.runs
WHERE project_id=$1
ORDER BY started_at DESC
LIMIT 1;
`, projectID).Scan(&runID)
	if err == nil && strings.TrimSpace(runID) != "" {
		return runID
	}
	err = db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, trace_id, pipeline_version, status, started_at)
VALUES ($1, $2::uuid, $3, 'running', now())
RETURNING run_id::text;
`, projectID, traceID, pipelineVersion).Scan(&runID)
	must(err)
	return runID
}

func newTraceIDHex32() string {
	return fmt.Sprintf("%032x", time.Now().UnixNano())[:32]
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("env %s is required", k)
	}
	return v
}
func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}
