package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type RuntimeRunDetailQueryPort interface {
	GetRuntimeRunDetail(ctx context.Context, projectID string, taskID int64) (run.RuntimeRunDetail, error)
}

type GetRuntimeRunDetailUseCase struct {
	Details RuntimeRunDetailQueryPort
}

type GetRuntimeRunDetailInput struct {
	ProjectID string
	TaskID    int64
}

type GetRuntimeRunDetailOutput struct {
	Detail run.RuntimeRunDetail
}

func (uc *GetRuntimeRunDetailUseCase) Handle(ctx context.Context, in GetRuntimeRunDetailInput) (GetRuntimeRunDetailOutput, error) {
	if uc.Details == nil {
		return GetRuntimeRunDetailOutput{}, fmt.Errorf("get runtime run detail: details query port is nil")
	}
	if in.ProjectID == "" {
		return GetRuntimeRunDetailOutput{}, fmt.Errorf("get runtime run detail: project_id is required")
	}
	if in.TaskID <= 0 {
		return GetRuntimeRunDetailOutput{}, fmt.Errorf("get runtime run detail: task_id is required")
	}

	detail, err := uc.Details.GetRuntimeRunDetail(ctx, in.ProjectID, in.TaskID)
	if err != nil {
		return GetRuntimeRunDetailOutput{}, fmt.Errorf("get runtime run detail query: %w", err)
	}

	return GetRuntimeRunDetailOutput{
		Detail: detail,
	}, nil
}