package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/internal/adminapi"
	"example.com/pisag_go/internal/worker"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	addr := getenv("AK_HTTP_ADDR", ":8083")

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
	normalizedRepo := postgres.NewNormalizedMultimodalResultRepo(db)
	reviewQueueRepo := postgres.NewMultimodalReviewQueueRepo(db)
	downstreamRepo := postgres.NewMultimodalDownstreamHandoffRepo(db)

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

	normalizeUC := &usecase.NormalizeV22MultimodalResultUseCase{
		Results:    resultRepo,
		Tasks:      taskRepo,
		Normalized: normalizedRepo,
	}

	enqueueReviewUC := &usecase.EnqueueV22MultimodalReviewUseCase{
		ReviewQueue: reviewQueueRepo,
		Normalized:  normalizedRepo,
	}

	downstreamUC := &usecase.CreateV22DownstreamHandoffUseCase{
		Downstream: downstreamRepo,
		Normalized: normalizedRepo,
	}

	executeOCRUC := &usecase.ExecuteV22OCRTaskUseCase{
		Tasks:               taskRepo,
		BudgetGate:          budgetGateUC,
		PolicyGate:          policyGateUC,
		MarkRunning:         markRunningUC,
		MarkSucceeded:       markSucceededUC,
		MarkReviewRequired:  markReviewRequiredUC,
		MarkFailedSoft:      markFailedSoftUC,
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     normalizeUC,
		EnqueueReview:       enqueueReviewUC,
		DownstreamHandoff:   downstreamUC,
		OCRPort:             &worker.StubOCRAdapter{},
	}

	executeVisionUC := &usecase.ExecuteV22VisionTaskUseCase{
		Tasks:               taskRepo,
		BudgetGate:          budgetGateUC,
		PolicyGate:          policyGateUC,
		MarkRunning:         markRunningUC,
		MarkSucceeded:       markSucceededUC,
		MarkReviewRequired:  markReviewRequiredUC,
		MarkFailedSoft:      markFailedSoftUC,
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     normalizeUC,
		EnqueueReview:       enqueueReviewUC,
		DownstreamHandoff:   downstreamUC,
		VisionPort:          &worker.StubVisionAdapter{},
	}

	handler := &adminapi.V22MultimodalHandler{
		CreateTaskUC:    &usecase.CreateMultimodalTaskUseCase{Tasks: taskRepo},
		AttachInputsUC:  &usecase.AttachMultimodalTaskInputsUseCase{Tasks: taskRepo, TaskInputs: taskInputRepo},
		ExecuteOCRUC:    executeOCRUC,
		ExecuteVisionUC: executeVisionUC,
		TaskRepo:        taskRepo,
		ResultRepo:      resultRepo,
		ReviewQueueRepo: reviewQueueRepo,
		NormalizedRepo:  normalizedRepo,
		DownstreamRepo:  downstreamRepo,
	}

	mux := http.NewServeMux()
	adminapi.RegisterV22MultimodalRoutes(mux, handler)

	log.Printf("v22 multimodal api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http serve: %v", err)
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}
