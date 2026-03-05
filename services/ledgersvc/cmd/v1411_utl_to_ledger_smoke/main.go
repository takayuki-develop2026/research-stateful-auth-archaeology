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
	"ledgersvc/usecase"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dsn := mustEnv("PG_DSN")
	projectID := mustEnv("PROJECT_ID")
	policyVersionID := getenvDefault("POLICY_VERSION_ID", "policy_v1_published")
	utlEventKey := mustEnv("UTL_EVENT_KEY")

	pool, err := pgxpool.New(ctx, dsn)
	dieIf(err, "pgxpool.New failed")
	defer pool.Close()

	ingestRepo := postgres.NewIngestRepoV141(pool)
	utlRepo := postgres.NewUtlRepoV6(pool)
	ledgerRepo := postgres.NewRepoV14(pool)

	uc := usecase.NewV1411UtlToLedgerSingleEvent(ingestRepo, utlRepo, ledgerRepo)

	runID := "run_v1411_" + nowKey()
	traceID := newTraceID()
	idemKey := "idem_v1411_" + nowKey()

	fmt.Println("[v1411_utl_to_ledger_smoke] ingest accept -> claim -> UTL get -> ledger post -> mark_succeeded")
	out, err := uc.RunOnce(ctx, usecase.V1411SmokeInput{
		ProjectID:       projectID,
		PolicyVersionID: policyVersionID,
		UTLEventKey:     utlEventKey,
		IdempotencyKey:  idemKey,
		RunID:           runID,
		TraceID:         traceID,
	})
	dieIf(err, "run_once failed")

	fmt.Printf("[v1411_utl_to_ledger_smoke] ✅ OK ingest_run_id=%s ledger_posting_id=%s ledger_posting_key=%s\n",
		out.IngestRunID, out.LedgerPostingID, out.LedgerPostingKey)
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
func nowKey() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
func newTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}