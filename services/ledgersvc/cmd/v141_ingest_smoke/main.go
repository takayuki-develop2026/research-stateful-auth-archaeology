package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgersvc/postgres"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	dsn := mustEnv("PG_DSN")
	projectID := getenvDefault("PROJECT_ID", "seed_project")
	policyVersionID := getenvDefault("POLICY_VERSION_ID", "policy_v1_published")

	pool, err := pgxpool.New(ctx, dsn)
	dieIf(err, "pgxpool.New failed")
	defer pool.Close()

	repo := postgres.NewIngestRepoV141(pool)

	fmt.Println("[v141_ingest_smoke] 1) accept(single_event) idempotent")
	runID := "run_ingest_" + nowKey()
	traceID := newTraceID()
	idemKey := "idem:" + nowKey()
	sourceEventKey := "utl:event:" + nowKey()

	a1, err := repo.Accept(ctx, postgres.IngestAcceptParams{
		ProjectID:       projectID,
		Mode:            "single_event",
		SourceEventKey:  sourceEventKey,
		FromTS:          nil,
		ToTS:            nil,
		Filter:          map[string]any{"note": "smoke"},
		IdempotencyKey:  idemKey,
		RunID:           runID,
		TraceID:         traceID,
		PolicyVersionID: policyVersionID,
		EvidenceRefs:    nil,
	})
	dieIf(err, "accept(single_event) failed")
	fmt.Printf("[v141_ingest_smoke]   accepted ingest_run_id=%s status=%s\n", a1.IngestRunID, a1.Status)

	// Accept again with same idempotency_key => same run id
	a2, err := repo.Accept(ctx, postgres.IngestAcceptParams{
		ProjectID:       projectID,
		Mode:            "single_event",
		SourceEventKey:  sourceEventKey,
		Filter:          map[string]any{"note": "smoke"},
		IdempotencyKey:  idemKey,
		RunID:           runID,
		TraceID:         traceID,
		PolicyVersionID: policyVersionID,
		EvidenceRefs:    []string{},
	})
	dieIf(err, "accept(single_event) second failed")
	assert(a2.IngestRunID == a1.IngestRunID, "expected same ingest_run_id for same idempotency_key")
	fmt.Printf("[v141_ingest_smoke]   idempotent OK ingest_run_id=%s status=%s\n", a2.IngestRunID, a2.Status)

	fmt.Println("[v141_ingest_smoke] 2) claim_next(project) => running")
	claimed, err := repo.ClaimNext(ctx, projectID)
	dieIf(err, "claim_next failed")
	assert(claimed != nil, "expected one claim")
	fmt.Printf("[v141_ingest_smoke]   claimed id=%s mode=%s source_event_key=%v\n", claimed.IngestRunID, claimed.Mode, claimed.SourceEventKey)

	fmt.Println("[v141_ingest_smoke] 3) touch()")
	dieIf(repo.Touch(ctx, claimed.IngestRunID), "touch failed")

	fmt.Println("[v141_ingest_smoke] 4) mark_succeeded()")
	stats := map[string]any{
		"event_count":          1,
		"posted_count":         0,
		"already_exists_count": 0,
		"failed_count":         0,
	}
	dieIf(repo.MarkSucceeded(ctx, claimed.IngestRunID, stats, []string{}), "mark_succeeded failed")
	fmt.Println("[v141_ingest_smoke]   mark_succeeded OK")

	// Claim again should return nil (no accepted remains with same idempotency)
	fmt.Println("[v141_ingest_smoke] 5) claim_next again => nil")
	claimed2, err := repo.ClaimNext(ctx, projectID)
	dieIf(err, "claim_next second failed")
	assert(claimed2 == nil, "expected nil claim when no accepted runs")

	fmt.Println("[v141_ingest_smoke] ✅ all OK")
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing env: %s\n", k)
		os.Exit(2)
	}
	return v
}
func getenvDefault(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func dieIf(err error, msg string) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", msg, err)
	os.Exit(1)
}
func assert(cond bool, msg string) {
	if cond {
		return
	}
	fmt.Fprintf(os.Stderr, "ASSERT FAILED: %s\n", msg)
	os.Exit(1)
}
func nowKey() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
func newTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}