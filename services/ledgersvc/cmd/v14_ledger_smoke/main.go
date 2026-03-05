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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dsn := mustEnv("PG_DSN")
	projectID := getenvDefault("PROJECT_ID", "proj_smoke")
	policyVersionID := getenvDefault("POLICY_VERSION_ID", "policy_v1_published")
	currency := getenvDefault("CURRENCY", "JPY")

	pool, err := pgxpool.New(ctx, dsn)
	dieIf(err, "pgxpool.New failed")
	defer pool.Close()

	repo := postgres.NewRepoV14(pool)
	uc := usecase.NewV14CreatePosting(repo)

	seedAccountsOrDie(ctx, repo, projectID, currency)

	fmt.Println("[v14_ledger_smoke] 1) idempotency + zero-sum OK")
	postingKey := "smoke:" + nowKey()
	sourceEventKey := "utl:" + nowKey()
	runID := "run_smoke_" + nowKey()
	traceID := newTraceID()

	linesOK := []postgres.EntryInput{
		{AccountKey: "platform:cash:clearing", Direction: "debit", Amount: 1000, Currency: currency, EntryKey: "line:1"},
		{AccountKey: "platform:revenue:sales", Direction: "credit", Amount: 1000, Currency: currency, EntryKey: "line:2"},
	}

	out1, err := uc.Handle(ctx, usecase.V14CreatePostingInput{
		ProjectID:       projectID,
		PostingKey:      postingKey,
		SourceEventKey:  sourceEventKey,
		PostingType:     "sale",
		Currency:        currency,
		PostedAt:        time.Now().UTC(),
		RunID:           runID,
		TraceID:         traceID,
		PolicyVersionID: policyVersionID,
		Lines:           linesOK,
	})
	dieIf(err, "create posting (first) failed")
	assert(out1.Status == "posted", "expected posted")
	assert(out1.DebitTotal == 1000 && out1.CreditTotal == 1000, "expected totals 1000/1000")
	fmt.Printf("[v14_ledger_smoke]   posted posting_id=%s totals=%d/%d\n", out1.PostingID, out1.DebitTotal, out1.CreditTotal)

	out2, err := uc.Handle(ctx, usecase.V14CreatePostingInput{
		ProjectID:       projectID,
		PostingKey:      postingKey,
		SourceEventKey:  sourceEventKey,
		PostingType:     "sale",
		Currency:        currency,
		PostedAt:        time.Now().UTC(),
		RunID:           runID,
		TraceID:         traceID,
		PolicyVersionID: policyVersionID,
		Lines:           linesOK,
	})
	dieIf(err, "create posting (second) failed")
	assert(out2.PostingID == out1.PostingID, "expected same posting_id")
	assert(out2.Status == "already_exists_posted" || out2.Status == "posted", "expected already_exists_posted or posted")
	fmt.Printf("[v14_ledger_smoke]   idempotent OK posting_id=%s status=%s\n", out2.PostingID, out2.Status)

	fmt.Println("[v14_ledger_smoke] 2) unknown account must fail (fail-closed)")
	_, err = uc.Handle(ctx, usecase.V14CreatePostingInput{
		ProjectID:       projectID,
		PostingKey:      "smoke:unknown:" + nowKey(),
		SourceEventKey:  "utl:unknown:" + nowKey(),
		PostingType:     "sale",
		Currency:        currency,
		PostedAt:        time.Now().UTC(),
		RunID:           "run_smoke_" + nowKey(),
		TraceID:         newTraceID(),
		PolicyVersionID: policyVersionID,
		Lines: []postgres.EntryInput{
			{AccountKey: "platform:cash:does_not_exist", Direction: "debit", Amount: 1000, Currency: currency, EntryKey: "line:1"},
			{AccountKey: "platform:revenue:sales", Direction: "credit", Amount: 1000, Currency: currency, EntryKey: "line:2"},
		},
	})
	assert(err != nil, "expected error for unknown account")
	fmt.Printf("[v14_ledger_smoke]   unknown account NG OK err=%v\n", err)

	fmt.Println("[v14_ledger_smoke] 3) zero-sum mismatch must fail")
	_, err = uc.Handle(ctx, usecase.V14CreatePostingInput{
		ProjectID:       projectID,
		PostingKey:      "smoke:zerosum:" + nowKey(),
		SourceEventKey:  "utl:zerosum:" + nowKey(),
		PostingType:     "sale",
		Currency:        currency,
		PostedAt:        time.Now().UTC(),
		RunID:           "run_smoke_" + nowKey(),
		TraceID:         newTraceID(),
		PolicyVersionID: policyVersionID,
		Lines: []postgres.EntryInput{
			{AccountKey: "platform:cash:clearing", Direction: "debit", Amount: 1000, Currency: currency, EntryKey: "line:1"},
			{AccountKey: "platform:revenue:sales", Direction: "credit", Amount: 999, Currency: currency, EntryKey: "line:2"},
		},
	})
	assert(err != nil, "expected error for zero-sum mismatch")
	fmt.Printf("[v14_ledger_smoke]   zero-sum NG OK err=%v\n", err)

	fmt.Println("[v14_ledger_smoke] 4) currency mismatch must fail")
	_, err = uc.Handle(ctx, usecase.V14CreatePostingInput{
		ProjectID:       projectID,
		PostingKey:      "smoke:currency:" + nowKey(),
		SourceEventKey:  "utl:currency:" + nowKey(),
		PostingType:     "sale",
		Currency:        currency,
		PostedAt:        time.Now().UTC(),
		RunID:           "run_smoke_" + nowKey(),
		TraceID:         newTraceID(),
		PolicyVersionID: policyVersionID,
		Lines: []postgres.EntryInput{
			{AccountKey: "platform:cash:clearing", Direction: "debit", Amount: 1000, Currency: "USD", EntryKey: "line:1"},
			{AccountKey: "platform:revenue:sales", Direction: "credit", Amount: 1000, Currency: currency, EntryKey: "line:2"},
		},
	})
	assert(err != nil, "expected error for currency mismatch")
	fmt.Printf("[v14_ledger_smoke]   currency mismatch NG OK err=%v\n", err)

	fmt.Println("[v14_ledger_smoke] ✅ all OK")
}

func seedAccountsOrDie(ctx context.Context, repo *postgres.RepoV14, projectID, currency string) {
	dieIf(repo.UpsertAccountForSmoke(ctx, projectID, "platform:cash:clearing", "asset", currency, "platform", ""), "seed cash")
	dieIf(repo.UpsertAccountForSmoke(ctx, projectID, "platform:revenue:sales", "revenue", currency, "platform", ""), "seed revenue")
}

// ---- utils ----

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