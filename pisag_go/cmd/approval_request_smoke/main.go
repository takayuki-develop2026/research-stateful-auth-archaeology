package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN")
	projectID := mustEnv("AK_PROJECT_ID")
	commitID := mustEnv("AK_COMMIT_ID")
	traceID := mustEnv("AK_TRACE_ID")

	requestedByType := strings.TrimSpace(os.Getenv("AK_REQUESTED_BY_TYPE"))
	requestedByID := strings.TrimSpace(os.Getenv("AK_REQUESTED_BY_ID"))
	reason := strings.TrimSpace(os.Getenv("AK_REASON"))

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	repo := postgres.NewApprovalRepository(db)
	uc := usecase.RequestApprovalUseCase{ApprovalRepo: repo}

	var rid *string
	if requestedByID != "" {
		rid = &requestedByID
	}
	var rsn *string
	if reason != "" {
		rsn = &reason
	}

	out, err := uc.Handle(ctx, usecase.RequestApprovalInput{
		ProjectID:       projectID,
		CommitID:        commitID,
		TraceID:         traceID,
		RequestedByType: requestedByType, // empty => default system
		RequestedByID:   rid,
		Reason:          rsn,
	})
	if err != nil {
		log.Fatalf("handle error: %v", err)
	}

	fmt.Printf("OK request_id=%s status=%s found_existing=%v\n", out.RequestID, out.Status, out.FoundExisting)
}