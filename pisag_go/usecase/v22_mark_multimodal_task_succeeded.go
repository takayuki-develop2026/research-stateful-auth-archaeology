package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskSucceededUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskSucceededInput struct {
	ProjectID  string
	TaskID     int64
	FinishedAt time.Time
}

type MarkMultimodalTaskSucceededOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskSucceededUseCase) Handle(ctx context.Context, in MarkMultimodalTaskSucceededInput) (MarkMultimodalTaskSucceededOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded: tasks repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded: task_id is required")
	}

	finishedAt := in.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	} else {
		finishedAt = finishedAt.UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded load task: %w", err)
	}
	if task.ProjectID != strings.TrimSpace(in.ProjectID) {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusRunning {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkSucceeded(ctx, strings.TrimSpace(in.ProjectID), in.TaskID, finishedAt)
	if err != nil {
		return MarkMultimodalTaskSucceededOutput{}, fmt.Errorf("mark multimodal task succeeded update: %w", err)
	}

	return MarkMultimodalTaskSucceededOutput{
		Task: updated,
	}, nil
}