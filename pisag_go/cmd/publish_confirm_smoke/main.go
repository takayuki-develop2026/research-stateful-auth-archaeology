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
	manifestID := mustEnv("AK_MANIFEST_ID")
	manifestHash := mustEnv("AK_MANIFEST_HASH")
	traceID := mustEnv("AK_TRACE_ID")

	runID := strings.TrimSpace(os.Getenv("AK_RUN_ID")) // optional

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	repo := postgres.NewPublishRepository(db)
	uc := usecase.PublishConfirmUseCase{PublishRepo: repo}

	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}

	out, err := uc.Handle(ctx, usecase.PublishConfirmInput{
		ProjectID:     "p1",
		ManifestID:    manifestID,
		ManifestHash:  manifestHash,
		TraceID:       traceID,
		RunID:         runIDPtr,
		Target:        "catalog_v1",
		AutoConfirm:   nil, // v4.6は通常false
		Meta: map[string]any{
			"smoke": true,
		},
	})
	if err != nil {
		log.Fatalf("handle error: %v", err)
	}

	fmt.Printf("OK commit_id=%s status=%s found_existing=%v commit_key=%s\n",
		out.CommitID, out.Status, out.FoundExisting, out.CommitKey)
}
