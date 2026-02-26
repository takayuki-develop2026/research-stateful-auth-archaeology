package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"strings"
	"time"
	"database/sql"


	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbURL := mustEnv("DATABASE_URL")
	db, err := postgres.Open(dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectID := firstNonEmpty(os.Getenv("PROJECT_ID"), mustQueryString(ctx, db, `SELECT id FROM public.projects ORDER BY id LIMIT 1`))
	evidenceID := mustQueryInt64(ctx, db, `SELECT id FROM public.evidence_assets ORDER BY id LIMIT 1`)

	v13repo := postgres.NewV13Repository(db)

	// -------- Idempotency
	idemUC := usecase.V13IdempotencyUseCase{V13Repo: v13repo}

	scope := "v13_smoke"
	key := "ak:idem:v13_smoke:" + shortHash("project="+projectID)

	start1, err := idemUC.Start(ctx, usecase.V13IdempotencyStartInput{
		ProjectID: projectID,
		Scope: scope,
		Key: key,
		RequestCanonical: "hello",
	})
	if err != nil { log.Fatalf("idem start1: %v", err) }
	log.Printf("[v13_smoke] idem start1: id=%d found=%v", start1.IdempotencyID, start1.FoundExisting)

	start2, err := idemUC.Start(ctx, usecase.V13IdempotencyStartInput{
		ProjectID: projectID,
		Scope: scope,
		Key: key,
		RequestCanonical: "hello",
	})
	if err != nil { log.Fatalf("idem start2: %v", err) }
	log.Printf("[v13_smoke] idem start2: id=%d found=%v", start2.IdempotencyID, start2.FoundExisting)

	summary := "ok"
	if err := idemUC.Finish(ctx, usecase.V13IdempotencyFinishInput{
		ProjectID: projectID,
		Id: start1.IdempotencyID,
		Status: "succeeded",
		Summary: &summary,
		ResultEvidenceAssetID: &evidenceID,
	}); err != nil { log.Fatalf("idem finish: %v", err) }
	log.Printf("[v13_smoke] idem finish: succeeded")

	// -------- DLQ
	dlqEnq := usecase.V13DlqEnqueueUseCase{V13Repo: v13repo}
	dlqMark := usecase.V13DlqMarkUseCase{V13Repo: v13repo}

	traceID := mustQueryString(ctx, db, `SELECT gen_random_uuid()::text`)
	taskType := "v13_smoke_task"
	source := "manual"

	dlqID, err := dlqEnq.Handle(ctx, usecase.V13DlqEnqueueInput{
		ProjectID: projectID,
		RunID: nil,
		TraceID: traceID,
		TaskType: taskType,
		Source: source,
		CorrelationKey: nil,
		PayloadEvidenceAssetID: evidenceID,
		LastErrorEvidenceAssetID: nil,
	})
	if err != nil { log.Fatalf("dlq enqueue: %v", err) }
	log.Printf("[v13_smoke] dlq enqueue: dlq_id=%d", dlqID)

	if err := dlqMark.Handle(ctx, usecase.V13DlqMarkInput{
		ProjectID: projectID,
		DlqID: dlqID,
		Status: "requeued",
		ResultErrorEvidenceAssetID: nil,
	}); err != nil { log.Fatalf("dlq mark requeued: %v", err) }
	log.Printf("[v13_smoke] dlq mark: requeued")

	if err := dlqMark.Handle(ctx, usecase.V13DlqMarkInput{
		ProjectID: projectID,
		DlqID: dlqID,
		Status: "resolved",
		ResultErrorEvidenceAssetID: nil,
	}); err != nil { log.Fatalf("dlq mark resolved: %v", err) }
	log.Printf("[v13_smoke] dlq mark: resolved")

	// -------- Compat contract
	ccUC := usecase.V13CompatContractUseCase{V13Repo: v13repo}
	sum := sha256.Sum256([]byte("openapi-v1"))
	check := hex.EncodeToString(sum[:])

	contractID, err := ccUC.Insert(ctx, usecase.V13CompatContractInsertInput{
		ProjectID: projectID,
		ContractType: "openapi",
		ContractVersion: "v1",
		ChecksumSha256: check,
		ArtifactRef: ptr("s3://dummy/openapi.json"),
		DiffSummary: ptr("v13 smoke"),
		DetailEvidenceAssetID: &evidenceID,
	})
	if err != nil { log.Fatalf("compat contract insert: %v", err) }
	log.Printf("[v13_smoke] compat_contract insert: id=%d", contractID)

	log.Printf("[v13_smoke] ✅ DONE")
}

func ptr(s string) *string { return &s }

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" { log.Fatalf("missing env: %s", k) }
	return v
}
func firstNonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v != "" { return v }
	return fallback
}
func mustQueryString(ctx context.Context, db *sql.DB, q string) string {
	var s string
	if err := db.QueryRowContext(ctx, q).Scan(&s); err != nil { log.Fatalf("query string: %v", err) }
	return strings.TrimSpace(s)
}
func mustQueryInt64(ctx context.Context, db *sql.DB, q string) int64 {
	var n int64
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil { log.Fatalf("query int64: %v", err) }
	if n <= 0 { log.Fatalf("query int64 returned %d", n) }
	return n
}
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}