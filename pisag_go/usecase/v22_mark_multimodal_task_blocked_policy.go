package usecase

import (
	"context"
	"fmt"
	"time"

	run "example.com/pisag_go/run"
)

type MarkMultimodalTaskBlockedPolicyUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type MarkMultimodalTaskBlockedPolicyInput struct {
	ProjectID                string
	TaskID                   int64
	FinishedAt               time.Time
	SoftErrorEvidenceAssetID *int64
}

type MarkMultimodalTaskBlockedPolicyOutput struct {
	Task run.MultimodalTask
}

func (uc *MarkMultimodalTaskBlockedPolicyUseCase) Handle(ctx context.Context, in MarkMultimodalTaskBlockedPolicyInput) (MarkMultimodalTaskBlockedPolicyOutput, error) {
	if uc.Tasks == nil {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy: tasks repository is nil")
	}
	if in.ProjectID == "" {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy: project_id is required")
	}
	if in.TaskID <= 0 {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy: task_id is required")
	}
	if in.FinishedAt.IsZero() {
		in.FinishedAt = time.Now().UTC()
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy: task project mismatch")
	}
	if task.Status != run.MultimodalTaskStatusRunning && task.Status != run.MultimodalTaskStatusQueued {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy: invalid current status=%s", task.Status)
	}

	updated, err := uc.Tasks.MarkBlockedPolicy(ctx, in.ProjectID, in.TaskID, in.FinishedAt.UTC(), in.SoftErrorEvidenceAssetID)
	if err != nil {
		return MarkMultimodalTaskBlockedPolicyOutput{}, fmt.Errorf("mark multimodal task blocked policy update: %w", err)
	}

	return MarkMultimodalTaskBlockedPolicyOutput{
		Task: updated,
	}, nil
}
