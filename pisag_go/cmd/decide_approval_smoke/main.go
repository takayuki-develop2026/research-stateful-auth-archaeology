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
	requestID := mustEnv("AK_REQUEST_ID")
	traceID := mustEnv("AK_TRACE_ID")

	decision := strings.TrimSpace(os.Getenv("AK_DECISION"))
	if decision == "" {
		decision = "approve"
	}

	decidedByType := strings.TrimSpace(os.Getenv("AK_DECIDED_BY_TYPE"))
	decidedByID := strings.TrimSpace(os.Getenv("AK_DECIDED_BY_ID"))
	comment := strings.TrimSpace(os.Getenv("AK_COMMENT"))

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	repo := postgres.NewApprovalRepository(db)
	uc := usecase.DecideApprovalUseCase{ApprovalRepo: repo}

	var did *string
	if decidedByID != "" {
		did = &decidedByID
	}
	var cmt *string
	if comment != "" {
		cmt = &comment
	}

	out, err := uc.Handle(ctx, usecase.DecideApprovalInput{
		ProjectID:     projectID,
		RequestID:     requestID,
		TraceID:       traceID,
		Decision:      decision, // approve/reject
		DecidedByType: decidedByType,
		DecidedByID:   did,
		Comment:       cmt,
	})
	if err != nil {
		log.Fatalf("handle error: %v", err)
	}

	fmt.Printf("OK request_id=%s decision=%s status=%s\n", out.RequestID, out.Decision, out.Status)
}
