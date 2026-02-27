package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func TestV13_IdempotencyAndDLQ(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var projectID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM public.projects ORDER BY id LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var evidenceID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM public.evidence_assets ORDER BY id LIMIT 1`).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}

	v13repo := postgres.NewV13Repository(db)
	idemUC := usecase.V13IdempotencyUseCase{V13Repo: v13repo}
	dlqEnq := usecase.V13DlqEnqueueUseCase{V13Repo: v13repo}
	dlqMark := usecase.V13DlqMarkUseCase{V13Repo: v13repo}

	start1, err := idemUC.Start(ctx, usecase.V13IdempotencyStartInput{
		ProjectID: projectID, Scope: "test", Key: "ak:idem:test:1", RequestCanonical: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	start2, err := idemUC.Start(ctx, usecase.V13IdempotencyStartInput{
		ProjectID: projectID, Scope: "test", Key: "ak:idem:test:1", RequestCanonical: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if start2.FoundExisting != true {
		t.Fatalf("expected found_existing=true on second start")
	}
	if start1.IdempotencyID != start2.IdempotencyID {
		t.Fatalf("expected same idempotency_id")
	}

	sum := "ok"
	if err := idemUC.Finish(ctx, usecase.V13IdempotencyFinishInput{
		ProjectID: projectID, Id: start1.IdempotencyID, Status: "succeeded", Summary: &sum, ResultEvidenceAssetID: &evidenceID,
	}); err != nil {
		t.Fatal(err)
	}

	traceID := "00000000-0000-0000-0000-000000000000"
	dlqID, err := dlqEnq.Handle(ctx, usecase.V13DlqEnqueueInput{
		ProjectID: projectID, TraceID: traceID, TaskType: "test_task", Source: "manual",
		PayloadEvidenceAssetID: evidenceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dlqID <= 0 {
		t.Fatalf("invalid dlq_id")
	}

	if err := dlqMark.Handle(ctx, usecase.V13DlqMarkInput{
		ProjectID: projectID, DlqID: dlqID, Status: "requeued",
	}); err != nil {
		t.Fatal(err)
	}

	if err := dlqMark.Handle(ctx, usecase.V13DlqMarkInput{
		ProjectID: projectID, DlqID: dlqID, Status: "resolved",
	}); err != nil {
		t.Fatal(err)
	}
}
