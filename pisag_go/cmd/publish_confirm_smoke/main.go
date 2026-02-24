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

func envBool(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN")
	manifestID := mustEnv("AK_MANIFEST_ID")
	manifestHash := mustEnv("AK_MANIFEST_HASH")
	traceID := mustEnv("AK_TRACE_ID")

	runID := strings.TrimSpace(os.Getenv("AK_RUN_ID")) // optional
	projectID := strings.TrimSpace(os.Getenv("AK_PROJECT_ID"))
	if projectID == "" {
		projectID = "p1"
	}

	auto := envBool("AK_AUTO_CONFIRM", false)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	publishRepo := postgres.NewPublishRepository(db)
	approvalRepo := postgres.NewApprovalRepository(db)

	uc := usecase.PublishConfirmUseCase{
		PublishRepo:  publishRepo,
		ApprovalRepo: approvalRepo, // ✅ v4.7 default-deny gate
	}

	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}

	out, err := uc.Handle(ctx, usecase.PublishConfirmInput{
		ProjectID:     projectID,
		ManifestID:    manifestID,
		ManifestHash:  manifestHash,
		TraceID:       traceID,
		RunID:         runIDPtr,
		Target:        "catalog_v1",
		AutoConfirm:   &auto, // ✅ envから渡す
		Meta: map[string]any{
			"smoke": true,
		},
	})
	if err != nil {
		log.Fatalf("handle error: %v", err)
	}

	fmt.Printf("OK commit_id=%s status=%s found_existing=%v commit_key=%s auto_confirm=%v\n",
		out.CommitID, out.Status, out.FoundExisting, out.CommitKey, auto)
}