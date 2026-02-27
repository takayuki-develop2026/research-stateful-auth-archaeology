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

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	projectID := mustEnv("AK_PROJECT_ID")
	dsn := mustEnv("DATABASE_URL")

	db := mustOpenDB(dsn)
	defer db.Close()

	traceID := newTraceIDHex32()
	runID := mustPickOrCreateRun(ctx, db, projectID, traceID, "v8.0")

	// -----------------------------
	// Source upsert (usecase)
	// -----------------------------
	// ★ここだけ、あなたの実装に合わせて ctor 名を調整
	// 例: postgres.NewDiscoverySourceRepository / postgres.NewDiscoveryRepository など
	srcRepo := postgres.NewDiscoverySourceRepository(db)

	uc := usecase.NewUpsertDiscoverySourceUsecase(srcRepo, "v8.0", "p8.0-default")

	in := run.DiscoverySourceUpsertInput{
		ProjectID:       projectID,
		RunID:           runID,
		TraceID:         traceID,
		PipelineVersion: "v8.0",
		PolicyVersion:   "p8.0-default",

		SourceType:   "pisag_html",
		SourceRefRaw: "https://example.com/pricing?utm_source=test",
		SourceRef:    "https://example.com/pricing",
		// SourceHash は空でOK（usecaseが計算）
	}

	r1, err := uc.Handle(ctx, in)
	must(err)
	log.Printf("[v8_smoke] upsert#1 source_id=%d found_existing=%v", r1.SourceID, r1.FoundExisting)

	r2, err := uc.Handle(ctx, in)
	must(err)
	log.Printf("[v8_smoke] upsert#2 source_id=%d found_existing=%v", r2.SourceID, r2.FoundExisting)

	if r1.FoundExisting {
		log.Fatalf("expected source found_existing=false on first upsert")
	}
	if !r2.FoundExisting {
		log.Fatalf("expected source found_existing=true on second upsert")
	}

	// -----------------------------
	// Candidate upsert (repo直叩き)
	// -----------------------------
	// ★ここも ctor 名をあなたの実装に合わせて調整
	cRepo := postgres.NewDiscoveryCandidateRepository(db)

	cKey := sha256hex("v8cand|" + projectID + "|catalog_source|https://example.com/pricing")

	cIn := run.DiscoveryCandidateUpsertInput{
		ProjectID:       projectID,
		SourceID:        r1.SourceID,
		RunID:           runID,
		TraceID:         traceID,
		PipelineVersion: "v8.0",
		PolicyVersion:   "p8.0-default",
		CandidateType:   "catalog_source",
		CandidateKey:    cKey,
	}

	c1, err := cRepo.Upsert(ctx, cIn)
	must(err)
	log.Printf("[v8_smoke] cand#1 id=%d found_existing=%v", c1.CandidateID, c1.FoundExisting)

	c2, err := cRepo.Upsert(ctx, cIn)
	must(err)
	log.Printf("[v8_smoke] cand#2 id=%d found_existing=%v", c2.CandidateID, c2.FoundExisting)

	if c1.FoundExisting {
		log.Fatalf("expected cand found_existing=false on first upsert")
	}
	if !c2.FoundExisting {
		log.Fatalf("expected cand found_existing=true on second upsert")
	}

	log.Printf("OK: v8 discovery smoke passed")
}

// ---------- helpers ----------

func mustOpenDB(dsn string) *sql.DB {
	db, err := sql.Open("pgx", dsn)
	must(err)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	must(db.Ping())
	return db
}

func mustPickOrCreateRun(ctx context.Context, db *sql.DB, projectID, traceID, pipelineVersion string) string {
	// 既存 run を拾う
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

	// 無ければ作る（runs_v41の想定）
	err = db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, trace_id, pipeline_version, status, started_at)
VALUES ($1, $2::uuid, $3, 'running', now())
RETURNING run_id::text;
`, projectID, traceID, pipelineVersion).Scan(&runID)
	must(err)
	return runID
}

func newTraceIDHex32() string {
	// 32hex（ハイフンなし）＝ Postgres uuidへ ::uuid キャスト可能
	return fmt.Sprintf("%032x", time.Now().UnixNano())[:32]
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
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
