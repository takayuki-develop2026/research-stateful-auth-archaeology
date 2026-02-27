package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN")
	projectID := mustEnv("AK_PROJECT_ID")

	traceID := envString("AK_TRACE_ID", fmt.Sprintf("trc_v18_smoke_%d", time.Now().Unix()))
	actorType := envString("AK_ACTOR_TYPE", "system")
	actorID := envString("AK_ACTOR_ID", "seed:local|kawada")

	taskType := envString("AK_TASK_TYPE", "fulltext_extract")
	pipelineVersion := envString("AK_PIPELINE_VERSION", "v18")
	policyVersionID := envStringPtr("AK_POLICY_VERSION_ID") // optional

	// ---- DB connect ----
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	// ---- repos ----
	evRepo := postgres.NewEvidenceV18Repository(db)
	arRepo := postgres.NewArtifactV18Repository(db)
	linkRepo := postgres.NewLinksV18Repository(db)
	contractRepo := postgres.NewTaskTypeContractV18Repository(db)

	// ---- run_id (create if missing) ----
	runID := envString("AK_RUN_ID", "")
	if strings.TrimSpace(runID) == "" {
		runID = mustCreateRun(ctx, db, projectID, pipelineVersion)
	}
	log.Printf("run_id=%s project_id=%s trace_id=%s actor=%s:%s", runID, projectID, traceID, actorType, actorID)

	// ---- 1) register contract evidence: output + input ----
	outRef := mustRegisterContractEvidence(ctx, evRepo, projectID, traceID, actorType, actorID,
		"contract://task_type/"+taskType+"/output",
		strings.Repeat("2", 64),
		20,
		"idem_contract_out_v18_smoke",
	)
	inRef := mustRegisterContractEvidence(ctx, evRepo, projectID, traceID, actorType, actorID,
		"contract://task_type/"+taskType+"/input",
		strings.Repeat("1", 64),
		10,
		"idem_contract_in_v18_smoke",
	)
	log.Printf("contract evidence: input=%s output=%s", inRef, outRef)

	// ---- 2) upsert task_type contract ----
	up := run.TaskTypeContractUpsertInput{
		ProjectID:                 projectID,
		TaskType:                  taskType,
		PipelineVersion:           pipelineVersion,
		PolicyVersionID:           policyVersionID,
		Enabled:                   true,
		InputContractEvidenceRef:  inRef,
		OutputContractEvidenceRef: outRef,
		DefaultMode:               strPtr("Mode0"),
		CreatedByType:             actorType,
		CreatedByID:               &actorID,
		TraceID:                   traceID,
		RunID:                     strPtr(runID),
		IdempotencyKey:            strPtr("idem_task_contract_v18_smoke"),
	}
	cres, err := contractRepo.Upsert(ctx, up)
	if err != nil {
		log.Fatalf("task_type contract upsert failed: %v", err)
	}
	log.Printf("task_type_contract_upsert: contract_id=%d change_kind=%s found_existing=%v",
		cres.ContractID, cres.ChangeKind, cres.FoundExisting)

	// ---- 3) register artifact ----
	ares, err := arRepo.Register(ctx, run.ArtifactRegisterInput{
		ProjectID:      projectID,
		ArtifactType:   "structured_json",
		SchemaVersion:  "extract.v1",
		ContentSHA256:  strPtr(strings.Repeat("a", 64)),
		ContentLength:  123,
		MimeType:       "application/json",
		Status:         "active",
		IdempotencyKey: "idem_artifact_v18_smoke",
	})
	if err != nil {
		log.Fatalf("artifact register failed: %v", err)
	}
	artifactRef := ares.ArtifactRef
	log.Printf("artifact_register: ref=%s found_existing=%v", artifactRef, ares.FoundExisting)

	// ---- 4) link run<->evidence (use input contract as sample evidence) ----
	l1, err := linkRepo.AddRunEvidenceLink(ctx, projectID, runID, inRef, "input", "idem_link_run_ev_001")
	if err != nil {
		log.Fatalf("AddRunEvidenceLink failed: %v", err)
	}
	l2, err := linkRepo.AddRunEvidenceLink(ctx, projectID, runID, inRef, "input", "idem_link_run_ev_002")
	if err != nil {
		log.Fatalf("AddRunEvidenceLink(2) failed: %v", err)
	}
	log.Printf("run_evidence_link: first=%+v second=%+v", l1, l2)

	// ---- 5) link run<->artifact ----
	ra1, err := linkRepo.AddRunArtifactLink(ctx, projectID, runID, artifactRef, "primary_output", "idem_link_run_art_001")
	if err != nil {
		log.Fatalf("AddRunArtifactLink failed: %v", err)
	}
	ra2, err := linkRepo.AddRunArtifactLink(ctx, projectID, runID, artifactRef, "primary_output", "idem_link_run_art_002")
	if err != nil {
		log.Fatalf("AddRunArtifactLink(2) failed: %v", err)
	}
	log.Printf("run_artifact_link: first=%+v second=%+v", ra1, ra2)

	// ---- 6) link artifact<->evidence ----
	ae1, err := linkRepo.AddArtifactEvidenceLink(ctx, projectID, artifactRef, inRef, "input", "idem_link_art_ev_001")
	if err != nil {
		log.Fatalf("AddArtifactEvidenceLink failed: %v", err)
	}
	ae2, err := linkRepo.AddArtifactEvidenceLink(ctx, projectID, artifactRef, inRef, "input", "idem_link_art_ev_002")
	if err != nil {
		log.Fatalf("AddArtifactEvidenceLink(2) failed: %v", err)
	}
	log.Printf("artifact_evidence_link: first=%+v second=%+v", ae1, ae2)

	log.Printf("OK v18 smoke: project=%s run=%s trace=%s", projectID, runID, traceID)
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}
func envString(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}
func envStringPtr(k string) *string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return nil
	}
	return &v
}
func strPtr(s string) *string { return &s }

func mustCreateRun(ctx context.Context, db *sql.DB, projectID, pipelineVersion string) string {
	var runID string
	err := db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, pipeline_version)
VALUES ($1, $2)
RETURNING run_id::text
`, projectID, pipelineVersion).Scan(&runID)
	if err != nil {
		log.Fatalf("create run failed: %v", err)
	}
	return runID
}

func mustRegisterContractEvidence(
	ctx context.Context,
	repo *postgres.EvidenceV18Repository,
	projectID, traceID, actorType, actorID, sourceURI, sha string,
	size int64,
	idem string,
) string {
	src := sourceURI
	lang := "en"
	res, err := repo.Register(ctx, run.EvidenceRegisterInput{
		ProjectID:       projectID,
		TraceID:         traceID,
		ActorType:       actorType,
		ActorID:         &actorID,
		MediaType:       "text",
		MimeType:        "application/json",
		SourceKind:      "generated",
		SourceURI:       &src,
		ContentSHA256:   sha,
		ContentLength:   size,
		Language:        &lang,
		RetentionPolicy: "standard",
		ExpiresAtUTC:    nil,
		IdempotencyKey:  idem,
	})
	if err != nil {
		log.Fatalf("evidence register failed: %v", err)
	}
	return res.EvidenceRef
}
