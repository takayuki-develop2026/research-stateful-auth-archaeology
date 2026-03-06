package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func TestV22MultimodalLifecycleIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV22TestPool(t)
	defer db.Close()

	projectID := "akproj_0000000000000000000"
	runID := "run_v22_it_" + uuid.NewString()
	traceID := "trace_v22_it_" + uuid.NewString()

	routerPlanEvidenceID := int64(109)
	optionsEvidenceID := int64(108)
	inputEvidenceID := int64(107)
	payloadEvidenceID := int64(106)
	outputEvidenceID := int64(105)
	policyDecisionID := int64(1)

	taskRepo := postgres.NewMultimodalTaskRepo(db)
	taskInputRepo := postgres.NewMultimodalTaskInputRepo(db)
	resultRepo := postgres.NewMultimodalResultRepo(db)
	resultOutputRepo := postgres.NewMultimodalResultOutputRepo(db)
	piiRepo := postgres.NewPIIRedactionRepo(db)

	createTaskUC := &usecase.CreateMultimodalTaskUseCase{Tasks: taskRepo}
	attachInputsUC := &usecase.AttachMultimodalTaskInputsUseCase{
		Tasks:      taskRepo,
		TaskInputs: taskInputRepo,
	}
	markRunningUC := &usecase.MarkMultimodalTaskRunningUseCase{Tasks: taskRepo}
	registerResultUC := &usecase.RegisterMultimodalResultUseCase{
		Tasks:   taskRepo,
		Results: resultRepo,
	}
	attachOutputsUC := &usecase.AttachMultimodalResultOutputsUseCase{
		Results:       resultRepo,
		ResultOutputs: resultOutputRepo,
	}
	registerPIIUC := &usecase.RegisterPIIRedactionUseCase{
		Redactions: piiRepo,
	}
	markSucceededUC := &usecase.MarkMultimodalTaskSucceededUseCase{
		Tasks: taskRepo,
	}

	inputHash, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeVision,
		PipelineVersion:  "v22-it-pipeline-1",
		PolicyVersionStr: "v21-it-policy-1",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: inputEvidenceID,
				SHA256:     "it_sha256_input_001",
				Kind:       "raw_image",
				Bytes:      2048,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "deterministic_mode=true|seed=7|temperature=0",
	})
	if err != nil {
		t.Fatalf("build input hash: %v", err)
	}

	createOut, err := createTaskUC.Handle(ctx, usecase.CreateMultimodalTaskInput{
		ProjectID:                 projectID,
		TraceID:                   traceID,
		RunID:                     runID,
		TaskType:                  run.MultimodalTaskTypeVision,
		PipelineVersion:           "v22-it-pipeline-1",
		PolicyVersionStr:          "v21-it-policy-1",
		InputHash:                 inputHash,
		RouterPlanEvidenceAssetID: routerPlanEvidenceID,
		OptionsEvidenceAssetID:    optionsEvidenceID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if createOut.Task.Status != run.MultimodalTaskStatusQueued {
		t.Fatalf("expected queued, got %s", createOut.Task.Status)
	}

	attachInOut, err := attachInputsUC.Handle(ctx, usecase.AttachMultimodalTaskInputsInput{
		ProjectID: projectID,
		TaskID:    createOut.Task.ID,
		Inputs: []run.AttachMultimodalTaskInputInput{
			{
				ProjectID:  projectID,
				TaskID:     createOut.Task.ID,
				EvidenceID: inputEvidenceID,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
	})
	if err != nil {
		t.Fatalf("attach inputs: %v", err)
	}
	if len(attachInOut.Inputs) != 1 {
		t.Fatalf("expected 1 input, got %d", len(attachInOut.Inputs))
	}

	runningOut, err := markRunningUC.Handle(ctx, usecase.MarkMultimodalTaskRunningInput{
		ProjectID: projectID,
		TaskID:    createOut.Task.ID,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if runningOut.Task.Status != run.MultimodalTaskStatusRunning {
		t.Fatalf("expected running, got %s", runningOut.Task.Status)
	}

	resultOut, err := registerResultUC.Handle(ctx, usecase.RegisterMultimodalResultInput{
		ProjectID:              projectID,
		TraceID:                traceID,
		RunID:                  runID,
		TaskID:                 createOut.Task.ID,
		ResultType:             run.MultimodalResultTypeVisionEntities,
		OutputHash:             "it_output_hash_001",
		PayloadEvidenceAssetID: payloadEvidenceID,
	})
	if err != nil {
		t.Fatalf("register result: %v", err)
	}

	attachOutOut, err := attachOutputsUC.Handle(ctx, usecase.AttachMultimodalResultOutputsInput{
		ProjectID: projectID,
		ResultID:  resultOut.Result.ID,
		Outputs: []run.AttachMultimodalResultOutputInput{
			{
				ProjectID:  projectID,
				ResultID:   resultOut.Result.ID,
				EvidenceID: outputEvidenceID,
				OutputRole: run.MultimodalOutputRoleAnnotatedImage,
				Seq:        0,
			},
		},
	})
	if err != nil {
		t.Fatalf("attach outputs: %v", err)
	}
	if len(attachOutOut.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(attachOutOut.Outputs))
	}

	piiOut, err := registerPIIUC.Handle(ctx, usecase.RegisterPIIRedactionInput{
		ProjectID:             projectID,
		TraceID:               traceID,
		EvidenceID:            outputEvidenceID,
		PolicyDecisionID:      policyDecisionID,
		RuleKey:               "pii.mask.email",
		Action:                run.PIIRedactionActionMask,
		AppliedByType:         run.PIIRedactionAppliedBySystem,
		DetailEvidenceAssetID: payloadEvidenceID,
	})
	if err != nil {
		t.Fatalf("register pii redaction: %v", err)
	}
	if piiOut.Redaction.Action != run.PIIRedactionActionMask {
		t.Fatalf("expected mask, got %s", piiOut.Redaction.Action)
	}

	succeededOut, err := markSucceededUC.Handle(ctx, usecase.MarkMultimodalTaskSucceededInput{
		ProjectID:  projectID,
		TaskID:     createOut.Task.ID,
		FinishedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	if succeededOut.Task.Status != run.MultimodalTaskStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", succeededOut.Task.Status)
	}
}

func TestV22MultimodalReviewRequiredAndFailedSoftIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV22TestPool(t)
	defer db.Close()

	projectID := "akproj_0000000000000000000"
	routerPlanEvidenceID := int64(109)
	optionsEvidenceID := int64(108)
	inputEvidenceID := int64(107)
	softErrorEvidenceID := int64(106)

	taskRepo := postgres.NewMultimodalTaskRepo(db)
	createTaskUC := &usecase.CreateMultimodalTaskUseCase{Tasks: taskRepo}
	markRunningUC := &usecase.MarkMultimodalTaskRunningUseCase{Tasks: taskRepo}
	markReviewRequiredUC := &usecase.MarkMultimodalTaskReviewRequiredUseCase{Tasks: taskRepo}
	markFailedSoftUC := &usecase.MarkMultimodalTaskFailedSoftUseCase{Tasks: taskRepo}

	inputHash1, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeVision,
		PipelineVersion:  "v22-it-pipeline-review",
		PolicyVersionStr: "v21-it-policy-review",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: inputEvidenceID,
				SHA256:     "review_sha256",
				Kind:       "raw_image",
				Bytes:      111,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "review=true",
	})
	if err != nil {
		t.Fatalf("build input hash1: %v", err)
	}

	reviewCreateOut, err := createTaskUC.Handle(ctx, usecase.CreateMultimodalTaskInput{
		ProjectID:                 projectID,
		TraceID:                   "trace_v22_review_" + uuid.NewString(),
		RunID:                     "run_v22_review_" + uuid.NewString(),
		TaskType:                  run.MultimodalTaskTypeVision,
		PipelineVersion:           "v22-it-pipeline-review",
		PolicyVersionStr:          "v21-it-policy-review",
		InputHash:                 inputHash1,
		RouterPlanEvidenceAssetID: routerPlanEvidenceID,
		OptionsEvidenceAssetID:    optionsEvidenceID,
	})
	if err != nil {
		t.Fatalf("create review task: %v", err)
	}

	_, err = markRunningUC.Handle(ctx, usecase.MarkMultimodalTaskRunningInput{
		ProjectID: projectID,
		TaskID:    reviewCreateOut.Task.ID,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark running for review task: %v", err)
	}

	reviewOut, err := markReviewRequiredUC.Handle(ctx, usecase.MarkMultimodalTaskReviewRequiredInput{
		ProjectID:                projectID,
		TaskID:                   reviewCreateOut.Task.ID,
		FinishedAt:               time.Now().UTC(),
		SoftErrorEvidenceAssetID: &softErrorEvidenceID,
	})
	if err != nil {
		t.Fatalf("mark review required: %v", err)
	}
	if reviewOut.Task.Status != run.MultimodalTaskStatusReviewRequired {
		t.Fatalf("expected review_required, got %s", reviewOut.Task.Status)
	}

	inputHash2, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeVision,
		PipelineVersion:  "v22-it-pipeline-failed",
		PolicyVersionStr: "v21-it-policy-failed",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: inputEvidenceID,
				SHA256:     "failed_sha256",
				Kind:       "raw_image",
				Bytes:      222,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "failed=true",
	})
	if err != nil {
		t.Fatalf("build input hash2: %v", err)
	}

	failedCreateOut, err := createTaskUC.Handle(ctx, usecase.CreateMultimodalTaskInput{
		ProjectID:                 projectID,
		TraceID:                   "trace_v22_failed_" + uuid.NewString(),
		RunID:                     "run_v22_failed_" + uuid.NewString(),
		TaskType:                  run.MultimodalTaskTypeVision,
		PipelineVersion:           "v22-it-pipeline-failed",
		PolicyVersionStr:          "v21-it-policy-failed",
		InputHash:                 inputHash2,
		RouterPlanEvidenceAssetID: routerPlanEvidenceID,
		OptionsEvidenceAssetID:    optionsEvidenceID,
	})
	if err != nil {
		t.Fatalf("create failed task: %v", err)
	}

	_, err = markRunningUC.Handle(ctx, usecase.MarkMultimodalTaskRunningInput{
		ProjectID: projectID,
		TaskID:    failedCreateOut.Task.ID,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark running for failed task: %v", err)
	}

	failedOut, err := markFailedSoftUC.Handle(ctx, usecase.MarkMultimodalTaskFailedSoftInput{
		ProjectID:                projectID,
		TaskID:                   failedCreateOut.Task.ID,
		FinishedAt:               time.Now().UTC(),
		SoftErrorEvidenceAssetID: &softErrorEvidenceID,
	})
	if err != nil {
		t.Fatalf("mark failed soft: %v", err)
	}
	if failedOut.Task.Status != run.MultimodalTaskStatusFailedSoft {
		t.Fatalf("expected failed_soft, got %s", failedOut.Task.Status)
	}
}

func openV22TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AK_PG_DSN")
	if dsn == "" {
		dsn = "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool new: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("pgxpool ping: %v", err)
	}
	return pool
}
