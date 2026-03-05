package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledgersvc/postgres"
	"ledgersvc/usecase"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dsn := mustEnv("PG_DSN")
	projectID := mustEnv("PROJECT_ID")
	policyVersionID := getenvDefault("POLICY_VERSION_ID", "policy_v1_published")

	from := getenvDefault("FROM_TS", time.Now().Add(-24*time.Hour).Format(time.RFC3339))
	to := getenvDefault("TO_TS", time.Now().Add(5*time.Minute).Format(time.RFC3339))

	fromTS, err := time.Parse(time.RFC3339, from)
	dieIf(err, "parse FROM_TS failed")
	toTS, err := time.Parse(time.RFC3339, to)
	dieIf(err, "parse TO_TS failed")

	statusFilter := getenvDefault("UTL_STATUS", "ingested")
	limit := 100

	pool, err := pgxpool.New(ctx, dsn)
	dieIf(err, "pgxpool.New failed")
	defer pool.Close()

	ingestRepo := postgres.NewIngestRepoV141(pool)
	utlRangeRepo := postgres.NewUtlRangeRepoV61(pool)
	utlStatusRepo := postgres.NewUtlStatusRepoV62(pool)
	ledgerRepo := postgres.NewRepoV14(pool)

	evidenceRepo := postgres.NewEvidenceRepoV18(pool)
	uc := usecase.NewV1412UtlToLedgerRange(ingestRepo, utlRangeRepo, utlStatusRepo, evidenceRepo, ledgerRepo)

	idemKey := "idem_v1412_" + fmt.Sprintf("%d", time.Now().UnixNano())

	// IMPORTANT: use UUID strings so DB functions expecting uuid can cast safely.
	runID := uuid.NewString()
	traceID := uuid.NewString()

	fmt.Println("[v1412_range_smoke] range ingest -> list UTL -> post ledger -> mark UTL processed")
	out, err := uc.RunOnce(ctx, usecase.V1412Input{
		ProjectID:       projectID,
		PolicyVersionID: policyVersionID,
		FromTS:          fromTS,
		ToTS:            toTS,
		StatusFilter:    &statusFilter,
		Limit:           limit,
		IdempotencyKey:  idemKey,
		RunID:           runID,
		TraceID:         traceID,
	})
	dieIf(err, "run_once failed")

	fmt.Printf("[v1412_range_smoke] ✅ OK ingest_run_id=%s processed=%d posted=%d already_exists=%d failed=%d utl_processed=%d utl_needs_retry=%d\n",
		out.IngestRunID, out.Processed, out.Posted, out.AlreadyExists, out.Failed, out.UtlMarkedProcessed, out.UtlMarkedNeedsRetry)
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