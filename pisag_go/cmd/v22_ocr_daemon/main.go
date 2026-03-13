package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/internal/worker"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	projectID := getenv("AK_PROJECT_ID", "")
	destinationKind := getenv("AK_DESTINATION_KIND", "provider_intelligence")
	pollInterval := mustDurationMS("AK_POLL_MS", 1000)

	if projectID == "" {
		log.Fatal("AK_PROJECT_ID is required")
	}

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
	runtimeUploadEvidenceRepo := postgres.NewRuntimeUploadEvidenceRepo(db)

	modelRunRepo := postgres.NewModelRunRepo(db)
	resultRepo := postgres.NewMultimodalResultRepo(db)
	resultOutputRepo := postgres.NewMultimodalResultOutputRepo(db)
	normalizedRepo := postgres.NewNormalizedMultimodalResultRepo(db)
	reviewQueueRepo := postgres.NewMultimodalReviewQueueRepo(db)
	downstreamRepo := postgres.NewMultimodalDownstreamHandoffRepo(db)
	runtimeEvidenceRepo := &postgres.RuntimeRequestEvidenceRepo{DB: db}

	markRunningUC := &usecase.MarkMultimodalTaskRunningUseCase{Tasks: taskRepo}
	markSucceededUC := &usecase.MarkMultimodalTaskSucceededUseCase{Tasks: taskRepo}
	markReviewRequiredUC := &usecase.MarkMultimodalTaskReviewRequiredUseCase{Tasks: taskRepo}
	markFailedSoftUC := &usecase.MarkMultimodalTaskFailedSoftUseCase{Tasks: taskRepo}
	markBlockedPolicyUC := &usecase.MarkMultimodalTaskBlockedPolicyUseCase{Tasks: taskRepo}
	markSkippedBudgetUC := &usecase.MarkMultimodalTaskSkippedBudgetUseCase{Tasks: taskRepo}

	budgetGateUC := &usecase.EvaluateV22BudgetGateUseCase{
		Tasks:              taskRepo,
		Gate:               &worker.ConventionBudgetGate{},
		MarkSkippedBudget:  markSkippedBudgetUC,
		MarkReviewRequired: markReviewRequiredUC,
	}
	policyGateUC := &usecase.EvaluateV22PolicyGateUseCase{
		Tasks:              taskRepo,
		Gate:               &worker.ConventionPolicyGate{},
		MarkBlockedPolicy:  markBlockedPolicyUC,
		MarkReviewRequired: markReviewRequiredUC,
	}

	execUC := &usecase.ExecuteV22OCRTaskUseCase{
		Tasks:              taskRepo,
		BudgetGate:         budgetGateUC,
		PolicyGate:         policyGateUC,
		MarkRunning:        markRunningUC,
		MarkSucceeded:      markSucceededUC,
		MarkReviewRequired: markReviewRequiredUC,
		MarkFailedSoft:     markFailedSoftUC,
		RegisterModelRun: &usecase.RegisterV22ModelRunUseCase{
			Tasks:     taskRepo,
			ModelRuns: modelRunRepo,
		},
		RegisterOCREvidence: &usecase.RegisterOCRResultEvidenceUseCase{
			Evidence: runtimeEvidenceRepo,
		},
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     &usecase.NormalizeV22MultimodalResultUseCase{Results: resultRepo, Tasks: taskRepo, Normalized: normalizedRepo},
		EnqueueReview:       &usecase.EnqueueV22MultimodalReviewUseCase{ReviewQueue: reviewQueueRepo, Normalized: normalizedRepo},
		DownstreamHandoff:   &usecase.CreateV22DownstreamHandoffUseCase{Downstream: downstreamRepo, Normalized: normalizedRepo},
		PreprocessPort: &worker.PythonPreprocessAdapter{
			BaseURL:           getenv("AK_PADDLEOCR_BASE_URL", ""),
			TaskInputs:        taskInputRepo,
			EvidenceSources:   runtimeUploadEvidenceRepo,
			EvidenceRegistrar: runtimeUploadEvidenceRepo,
			EvidenceStore:     worker.NewFSEvidenceStore(getenv("AK_EVIDENCE_DIR", "./var/evidence")),
			BaseDir:           getenv("AK_EVIDENCE_DIR", "./var/evidence"),
		},
		OCRPort: &worker.PaddleOCRAdapter{
			BaseURL:           getenv("AK_PADDLEOCR_BASE_URL", ""),
			TaskInputs:        taskInputRepo,
			EvidenceSources:   runtimeUploadEvidenceRepo,
			AllowStubFallback: true,
		},
	}

	log.Printf("[v22_ocr_daemon] start project_id=%s poll=%s", projectID, pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[v22_ocr_daemon] stop: %v", ctx.Err())
			return
		default:
		}

		task, ok, err := taskRepo.ClaimNextQueuedOCRTask(ctx, projectID)
		if err != nil {
			log.Printf("[v22_ocr_daemon] claim error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
				continue
			}
		}

		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		log.Printf("[v22_ocr_daemon] claimed task_id=%d run_id=%s", task.ID, task.RunID)

		out, err := execUC.Handle(ctx, usecase.ExecuteV22OCRTaskInput{
			ProjectID:       projectID,
			TaskID:          task.ID,
			DestinationKind: destinationKind,
		})
		if err != nil {
			log.Printf("[v22_ocr_daemon] execute error task_id=%d: %v", task.ID, err)
			continue
		}

		log.Printf("[v22_ocr_daemon] completed task_id=%d status=%s", out.Task.ID, out.Task.Status)
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func mustDurationMS(k string, fallback int) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return time.Duration(fallback) * time.Millisecond
	}
	ms, err := time.ParseDuration(v + "ms")
	if err == nil {
		return ms
	}
	log.Printf("[v22_ocr_daemon] invalid %s=%q, fallback=%dms", k, v, fallback)
	return time.Duration(fallback) * time.Millisecond
}

func isNoRowsLike(err error) bool {
	return err != nil && errors.Is(err, context.Canceled)
}