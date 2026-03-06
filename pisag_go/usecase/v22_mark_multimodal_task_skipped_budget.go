package usecase

import (
	"context"
	"fmt"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskSkippedBudgetUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskSkippedBudgetInput struct {
	ProjectID                string
	TaskID                   int64
	FinishedAt               time.Time
	SoftErrorEvidenceAssetID *int64
}

type MarkMultimodalTaskSkippedBudgetOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskSkippedBudgetUseCase) Handle(ctx context.Context, in MarkMultimodalTaskSkippedBudgetInput) (MarkMultimodalTaskSkippedBudgetOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget: tasks repository is nil")
	}
	if in.ProjectID == "" {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget: task_id is required")
	}
	if in.FinishedAt.IsZero() {
		in.FinishedAt = time.Now().UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusRunning && task.Status != run.MultimodalTaskStatusQueued {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkSkippedBudget(ctx, in.ProjectID, in.TaskID, in.FinishedAt.UTC(), in.SoftErrorEvidenceAssetID)
	if err != nil {
		return MarkMultimodalTaskSkippedBudgetOutput{}, fmt.Errorf("mark multimodal task skipped budget update: %w", err)
	}

	return MarkMultimodalTaskSkippedBudgetOutput{
		Task: updated,
	}, nil
}
