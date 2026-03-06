package usecase

import (
	"context"
	"fmt"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskRunningUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskRunningInput struct {
	ProjectID string
	TaskID    int64
	StartedAt time.Time
}

type MarkMultimodalTaskRunningOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskRunningUseCase) Handle(ctx context.Context, in MarkMultimodalTaskRunningInput) (MarkMultimodalTaskRunningOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running: tasks repository is nil")
	}
	if in.ProjectID == "" {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running: task_id is required")
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = time.Now().UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusQueued {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkRunning(ctx, in.ProjectID, in.TaskID, in.StartedAt.UTC())
	if err != nil {
		return MarkMultimodalTaskRunningOutput{}, fmt.Errorf("mark multimodal task running update: %w", err)
	}

	return MarkMultimodalTaskRunningOutput{
		Task: updated,
	}, nil
}
