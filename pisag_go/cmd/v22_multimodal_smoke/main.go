package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	projectID := getenv("AK_PROJECT_ID", "default")
	runID := getenv("AK_RUN_ID", "run_v22_smoke_001")
	traceID := getenv("AK_TRACE_ID", "trace_v22_smoke_001")

	// final status:
	// - succeeded
	// - review_required
	// - failed_soft
	finalStatus := strings.TrimSpace(getenv("AK_FINAL_STATUS", "succeeded"))

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}

	taskRepo := postgres.NewMultimodalTaskRepo(db)
	taskInputRepo := postgres.NewMultimodalTaskInputRepo(db)
	resultRepo := postgres.NewMultimodalResultRepo(db)
	resultOutputRepo := postgres.NewMultimodalResultOutputRepo(db)
	piiRepo := postgres.NewPIIRedactionRepo(db)

	createTaskUC := &usecase.CreateMultimodalTaskUseCase{
		Tasks: taskRepo,
	}
	attachInputsUC := &usecase.AttachMultimodalTaskInputsUseCase{
		Tasks:      taskRepo,
		TaskInputs: taskInputRepo,
	}
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
	markRunningUC := &usecase.MarkMultimodalTaskRunningUseCase{
		Tasks: taskRepo,
	}
	markSucceededUC := &usecase.MarkMultimodalTaskSucceededUseCase{
		Tasks: taskRepo,
	}
	markReviewRequiredUC := &usecase.MarkMultimodalTaskReviewRequiredUseCase{
		Tasks: taskRepo,
	}
	markFailedSoftUC := &usecase.MarkMultimodalTaskFailedSoftUseCase{
		Tasks: taskRepo,
	}

	// 既存 evidence_assets の実IDを使う
	routerPlanEvidenceID := mustInt64Env("AK_ROUTER_PLAN_EVIDENCE_ID", 1)
	optionsEvidenceID := mustInt64Env("AK_OPTIONS_EVIDENCE_ID", 1)
	inputEvidenceID := mustInt64Env("AK_INPUT_EVIDENCE_ID", 1)
	payloadEvidenceID := mustInt64Env("AK_PAYLOAD_EVIDENCE_ID", 1)
	confidenceEvidenceID := mustOptionalInt64Env("AK_CONFIDENCE_EVIDENCE_ID")
	outputEvidenceID := mustInt64Env("AK_OUTPUT_EVIDENCE_ID", 1)
	redactionEvidenceID := mustInt64Env("AK_REDACTION_EVIDENCE_ID", outputEvidenceID)
	policyDecisionID := mustInt64Env("AK_POLICY_DECISION_ID", 1)
	softErrorEvidenceID := mustOptionalInt64Env("AK_SOFT_ERROR_EVIDENCE_ID")

	inputHash, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeVision,
		PipelineVersion:  "v22-smoke-pipeline-1",
		PolicyVersionStr: "v21-smoke-policy-1",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: inputEvidenceID,
				SHA256:     "smoke_sha256_input_001",
				Kind:       "raw_image",
				Bytes:      1024,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "deterministic_mode=true|seed=42|temperature=0",
	})
	if err != nil {
		log.Fatalf("build input hash: %v", err)
	}

	createTaskOut, err := createTaskUC.Handle(ctx, usecase.CreateMultimodalTaskInput{
		ProjectID:                 projectID,
		TraceID:                   traceID,
		RunID:                     runID,
		TaskType:                  run.MultimodalTaskTypeVision,
		PipelineVersion:           "v22-smoke-pipeline-1",
		PolicyVersionStr:          "v21-smoke-policy-1",
		InputHash:                 inputHash,
		RouterPlanEvidenceAssetID: routerPlanEvidenceID,
		OptionsEvidenceAssetID:    optionsEvidenceID,
	})
	if err != nil {
		log.Fatalf("create multimodal task: %v", err)
	}

	fmt.Println("== create multimodal task OK ==")
	fmt.Printf("task_id=%d task_key=%s created=%v status=%s\n",
		createTaskOut.Task.ID,
		createTaskOut.Task.TaskKey,
		createTaskOut.Created,
		createTaskOut.Task.Status,
	)

	attachInputsOut, err := attachInputsUC.Handle(ctx, usecase.AttachMultimodalTaskInputsInput{
		ProjectID: projectID,
		TaskID:    createTaskOut.Task.ID,
		Inputs: []run.AttachMultimodalTaskInputInput{
			{
				ProjectID:  projectID,
				TaskID:     createTaskOut.Task.ID,
				EvidenceID: inputEvidenceID,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
	})
	if err != nil {
		log.Fatalf("attach multimodal task inputs: %v", err)
	}

	fmt.Println("== attach task inputs OK ==")
	fmt.Printf("task_id=%d inputs=%d\n", attachInputsOut.Task.ID, len(attachInputsOut.Inputs))

	runningOut, err := markRunningUC.Handle(ctx, usecase.MarkMultimodalTaskRunningInput{
		ProjectID: projectID,
		TaskID:    createTaskOut.Task.ID,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Fatalf("mark multimodal task running: %v", err)
	}

	fmt.Println("== mark running OK ==")
	fmt.Printf("task_id=%d status=%s\n", runningOut.Task.ID, runningOut.Task.Status)

	registerResultOut, err := registerResultUC.Handle(ctx, usecase.RegisterMultimodalResultInput{
		ProjectID:                 projectID,
		TraceID:                   traceID,
		RunID:                     runID,
		TaskID:                    createTaskOut.Task.ID,
		ResultType:                run.MultimodalResultTypeVisionEntities,
		OutputHash:                "smoke_output_hash_001",
		PayloadEvidenceAssetID:    payloadEvidenceID,
		ConfidenceEvidenceAssetID: confidenceEvidenceID,
	})
	if err != nil {
		log.Fatalf("register multimodal result: %v", err)
	}

	fmt.Println("== register multimodal result OK ==")
	fmt.Printf("result_id=%d result_key=%s created=%v\n",
		registerResultOut.Result.ID,
		registerResultOut.Result.ResultKey,
		registerResultOut.Created,
	)

	attachOutputsOut, err := attachOutputsUC.Handle(ctx, usecase.AttachMultimodalResultOutputsInput{
		ProjectID: projectID,
		ResultID:  registerResultOut.Result.ID,
		Outputs: []run.AttachMultimodalResultOutputInput{
			{
				ProjectID:  projectID,
				ResultID:   registerResultOut.Result.ID,
				EvidenceID: outputEvidenceID,
				OutputRole: run.MultimodalOutputRoleAnnotatedImage,
				Seq:        0,
			},
		},
	})
	if err != nil {
		log.Fatalf("attach multimodal result outputs: %v", err)
	}

	fmt.Println("== attach result outputs OK ==")
	fmt.Printf("result_id=%d outputs=%d\n", attachOutputsOut.Result.ID, len(attachOutputsOut.Outputs))

	registerPIIOut, err := registerPIIUC.Handle(ctx, usecase.RegisterPIIRedactionInput{
		ProjectID:             projectID,
		TraceID:               traceID,
		EvidenceID:            redactionEvidenceID,
		PolicyDecisionID:      policyDecisionID,
		RuleKey:               "pii.mask.email",
		Action:                run.PIIRedactionActionMask,
		AppliedByType:         run.PIIRedactionAppliedBySystem,
		AppliedByID:           "",
		DetailEvidenceAssetID: payloadEvidenceID,
	})
	if err != nil {
		log.Fatalf("register pii redaction: %v", err)
	}

	fmt.Println("== register pii redaction OK ==")
	fmt.Printf("pii_redaction_id=%d action=%s rule_key=%s\n",
		registerPIIOut.Redaction.ID,
		registerPIIOut.Redaction.Action,
		registerPIIOut.Redaction.RuleKey,
	)

	switch finalStatus {
	case "succeeded":
		succeededOut, err := markSucceededUC.Handle(ctx, usecase.MarkMultimodalTaskSucceededInput{
			ProjectID:  projectID,
			TaskID:     createTaskOut.Task.ID,
			FinishedAt: time.Now().UTC(),
		})
		if err != nil {
			log.Fatalf("mark multimodal task succeeded: %v", err)
		}
		fmt.Println("== mark succeeded OK ==")
		fmt.Printf("task_id=%d status=%s\n", succeededOut.Task.ID, succeededOut.Task.Status)

	case "review_required":
		reviewOut, err := markReviewRequiredUC.Handle(ctx, usecase.MarkMultimodalTaskReviewRequiredInput{
			ProjectID:                projectID,
			TaskID:                   createTaskOut.Task.ID,
			FinishedAt:               time.Now().UTC(),
			SoftErrorEvidenceAssetID: softErrorEvidenceID,
		})
		if err != nil {
			log.Fatalf("mark multimodal task review required: %v", err)
		}
		fmt.Println("== mark review required OK ==")
		fmt.Printf("task_id=%d status=%s\n", reviewOut.Task.ID, reviewOut.Task.Status)

	case "failed_soft":
		failedOut, err := markFailedSoftUC.Handle(ctx, usecase.MarkMultimodalTaskFailedSoftInput{
			ProjectID:                projectID,
			TaskID:                   createTaskOut.Task.ID,
			FinishedAt:               time.Now().UTC(),
			SoftErrorEvidenceAssetID: softErrorEvidenceID,
		})
		if err != nil {
			log.Fatalf("mark multimodal task failed soft: %v", err)
		}
		fmt.Println("== mark failed soft OK ==")
		fmt.Printf("task_id=%d status=%s\n", failedOut.Task.ID, failedOut.Task.Status)

	default:
		log.Fatalf("unsupported AK_FINAL_STATUS=%q (allowed: succeeded, review_required, failed_soft)", finalStatus)
	}

	fmt.Println("== v22 multimodal smoke completed successfully ==")
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func mustInt64Env(k string, fallback int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	var out int64
	_, err := fmt.Sscan(v, &out)
	if err != nil {
		log.Fatalf("invalid int64 env %s=%q: %v", k, v, err)
	}
	return out
}

func mustOptionalInt64Env(k string) *int64 {
	v := os.Getenv(k)
	if v == "" {
		return nil
	}
	var out int64
	_, err := fmt.Sscan(v, &out)
	if err != nil {
		log.Fatalf("invalid optional int64 env %s=%q: %v", k, v, err)
	}
	return &out
}
