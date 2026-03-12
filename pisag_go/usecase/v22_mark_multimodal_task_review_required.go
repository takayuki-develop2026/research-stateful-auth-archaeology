package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskReviewRequiredUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskReviewRequiredInput struct {
	ProjectID                string
	TaskID                   int64
	FinishedAt               time.Time
	SoftErrorEvidenceAssetID *int64
}

type MarkMultimodalTaskReviewRequiredOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskReviewRequiredUseCase) Handle(ctx context.Context, in MarkMultimodalTaskReviewRequiredInput) (MarkMultimodalTaskReviewRequiredOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required: tasks repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required: task_id is required")
	}

	finishedAt := in.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	} else {
		finishedAt = finishedAt.UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required load task: %w", err)
	}
	if task.ProjectID != strings.TrimSpace(in.ProjectID) {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusRunning && task.Status != run.MultimodalTaskStatusQueued {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkReviewRequired(ctx, strings.TrimSpace(in.ProjectID), in.TaskID, finishedAt, in.SoftErrorEvidenceAssetID)
	if err != nil {
		return MarkMultimodalTaskReviewRequiredOutput{}, fmt.Errorf("mark multimodal task review required update: %w", err)
	}

	return MarkMultimodalTaskReviewRequiredOutput{
		Task: updated,
	}, nil
}