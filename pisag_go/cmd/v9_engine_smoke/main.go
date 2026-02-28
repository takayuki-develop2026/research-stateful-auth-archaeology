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
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("DATABASE_URL")
	projectID := getenv("AK_PROJECT_ID", "akproj_0000000000000000000")

	db := mustOpenDB(dsn)
	defer db.Close()

	// You already have v18 evidence repo in postgres/*
	evidenceRepo := postgres.NewEvidenceV18Repository(db)
	engineRepo := postgres.NewEngineRepoV9(db)

	uc := usecase.NewEngineDecideUsecaseV9(engineRepo, evidenceRepo, nil)
	uc.CacheTTL = 2 * time.Minute

	// Create run_id/trace_id for the request (v3)
	traceID1 := mustUUIDText(ctx, db)
	runID1 := mustCreateRun(ctx, db, projectID, traceID1, "v9")

	// Force cache-miss for this execution by using a unique nonce.
	nonce := time.Now().UnixNano()
	inJSON := []byte(fmt.Sprintf(`{"hello":"world","n":1,"nonce":%d}`, nonce))

	in1 := usecase.EngineDecideInput{
		ProjectID: projectID,
		RunID:     runID1,
		TraceID:   traceID1,

		TaskType:        "routing_decide_v5", // v9 P0 builder returns "plan"
		Mode:            "mode0_rule_only",
		PipelineVersion: "v9",
		PolicyVersion:   "p9-default",

		IdempotencyKey: fmt.Sprintf("engine_decide:v9_smoke:%d:1", nonce),

		Principal: usecase.EnginePrincipal{
			ActorType: "system",
			ActorID:   "smoke",
			Roles:     []string{"developer"},
		},

		InputJSON: inJSON,
	}

	out1, err := uc.Handle(ctx, in1)
	must(err)
	log.Printf("[v9_smoke] #1 engine_run_id=%s status=%s cache_hit=%v decision_id=%s decision_type=%s cache_key=%s",
		out1.EngineRunID, out1.Status, out1.CacheHit, out1.DecisionID, out1.DecisionType, out1.CacheKey)

	if out1.CacheHit {
		log.Fatalf("expected cache_hit=false on first call (nonce=%d)", nonce)
	}

	// second run (different run_id/trace_id, SAME input/principal/policy -> should hit cache)
	traceID2 := mustUUIDText(ctx, db)
	runID2 := mustCreateRun(ctx, db, projectID, traceID2, "v9")

	in2 := in1
	in2.RunID = runID2
	in2.TraceID = traceID2
	in2.IdempotencyKey = fmt.Sprintf("engine_decide:v9_smoke:%d:2", nonce)

	out2, err := uc.Handle(ctx, in2)
	must(err)
	log.Printf("[v9_smoke] #2 engine_run_id=%s status=%s cache_hit=%v decision_id=%s decision_type=%s cache_key=%s",
		out2.EngineRunID, out2.Status, out2.CacheHit, out2.DecisionID, out2.DecisionType, out2.CacheKey)

	if !out2.CacheHit {
		log.Fatalf("expected cache_hit=true on second call (nonce=%d)", nonce)
	}
	if out1.CacheKey != out2.CacheKey {
		log.Fatalf("expected same cache_key, got %s vs %s (nonce=%d)", out1.CacheKey, out2.CacheKey, nonce)
	}
	if out1.DecisionID != out2.DecisionID {
		log.Fatalf("expected same decision_id (cached), got %s vs %s (nonce=%d)", out1.DecisionID, out2.DecisionID, nonce)
	}

	log.Printf("OK: v9 engine smoke passed (nonce=%d)", nonce)
}

// ----------------------------- helpers -----------------------------

func mustOpenDB(dsn string) *sql.DB {
	db, err := sql.Open("pgx", dsn)
	must(err)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	must(db.Ping())
	return db
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("env %s is required", k)
	}
	return v
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustUUIDText(ctx context.Context, db *sql.DB) string {
	var s string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text;`).Scan(&s); err != nil {
		log.Fatal(err)
	}
	return s
}

func mustCreateRun(ctx context.Context, db *sql.DB, projectID, traceID, pipelineVersion string) string {
	var runID string
	err := db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, trace_id, pipeline_version, status, started_at)
VALUES ($1, $2::uuid, $3, 'running', now())
RETURNING run_id::text;
`, projectID, traceID, pipelineVersion).Scan(&runID)
	must(err)
	return runID
}
