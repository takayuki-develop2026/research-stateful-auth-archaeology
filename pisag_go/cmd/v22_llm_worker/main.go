package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/internal/worker"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
	run "example.com/pisag_go/run"
)

func main() {
	ctx := context.Background()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	projectID := getenv("AK_PROJECT_ID", "")
	taskID := mustInt64Env("AK_TASK_ID")
	taskKind := getenv("AK_LLM_TASK_KIND", "summarize")
	destinationKind := getenv("AK_DESTINATION_KIND", "provider_intelligence")

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

	execUC := &usecase.ExecuteV22LLMTaskUseCase{
		Tasks:               taskRepo,
		BudgetGate:          budgetGateUC,
		PolicyGate:          policyGateUC,
		MarkRunning:         markRunningUC,
		MarkSucceeded:       markSucceededUC,
		MarkReviewRequired:  markReviewRequiredUC,
		MarkFailedSoft:      markFailedSoftUC,
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     &usecase.NormalizeV22MultimodalResultUseCase{Results: resultRepo, Tasks: taskRepo, Normalized: normalizedRepo},
		EnqueueReview:       &usecase.EnqueueV22MultimodalReviewUseCase{ReviewQueue: reviewQueueRepo, Normalized: normalizedRepo},
		DownstreamHandoff:   &usecase.CreateV22DownstreamHandoffUseCase{Downstream: downstreamRepo, Normalized: normalizedRepo},
		LLMPort:             &worker.StubLLMAdapter{},
	}

	out, err := execUC.Handle(ctx, usecase.ExecuteV22LLMTaskInput{
		ProjectID:       projectID,
		TaskID:          taskID,
		TaskKind:        run.LLMTaskKind(taskKind),
		Context:         map[string]any{},
		DestinationKind: destinationKind,
	})
	if err != nil {
		log.Fatalf("execute llm task: %v", err)
	}

	log.Printf("v22 llm worker completed: task_id=%d status=%s", out.Task.ID, out.Task.Status)
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func mustInt64Env(k string) int64 {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", k, v, err)
	}
	return n
}