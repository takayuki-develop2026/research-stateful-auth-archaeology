package usecase

import (
	"context"
	"fmt"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskFailedSoftUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskFailedSoftInput struct {
	ProjectID                string
	TaskID                   int64
	FinishedAt               time.Time
	SoftErrorEvidenceAssetID *int64
}

type MarkMultimodalTaskFailedSoftOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskFailedSoftUseCase) Handle(ctx context.Context, in MarkMultimodalTaskFailedSoftInput) (MarkMultimodalTaskFailedSoftOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft: tasks repository is nil")
	}
	if in.ProjectID == "" {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft: task_id is required")
	}
	if in.FinishedAt.IsZero() {
		in.FinishedAt = time.Now().UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusRunning && task.Status != run.MultimodalTaskStatusQueued {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkFailedSoft(ctx, in.ProjectID, in.TaskID, in.FinishedAt.UTC(), in.SoftErrorEvidenceAssetID)
	if err != nil {
		return MarkMultimodalTaskFailedSoftOutput{}, fmt.Errorf("mark multimodal task failed soft update: %w", err)
	}

	return MarkMultimodalTaskFailedSoftOutput{
		Task: updated,
	}, nil
}
