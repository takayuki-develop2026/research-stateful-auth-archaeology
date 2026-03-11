package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type ExecuteV22EmbeddingTaskUseCase struct {
	Tasks              run.MultimodalTaskRepository
	BudgetGate         *EvaluateV22BudgetGateUseCase
	PolicyGate         *EvaluateV22PolicyGateUseCase
	MarkRunning        *MarkMultimodalTaskRunningUseCase
	MarkSucceeded      *MarkMultimodalTaskSucceededUseCase
	MarkReviewRequired *MarkMultimodalTaskReviewRequiredUseCase
	MarkFailedSoft     *MarkMultimodalTaskFailedSoftUseCase

	RegisterResult      *RegisterMultimodalResultUseCase
	AttachResultOutputs *AttachMultimodalResultOutputsUseCase
	NormalizeResult     *NormalizeV22MultimodalResultUseCase
	EnqueueReview       *EnqueueV22MultimodalReviewUseCase
	DownstreamHandoff   *CreateV22DownstreamHandoffUseCase

	EmbeddingPort run.EmbeddingExecutionPort
}

type ExecuteV22EmbeddingTaskInput struct {
	ProjectID       string
	TaskID          int64
	DestinationKind string
}

type ExecuteV22EmbeddingTaskOutput struct {
	Task             run.MultimodalTask
	Result           *run.MultimodalResult
	NormalizedResult *run.NormalizedMultimodalResult
}

func (uc *ExecuteV22EmbeddingTaskUseCase) Handle(ctx context.Context, in ExecuteV22EmbeddingTaskInput) (ExecuteV22EmbeddingTaskOutput, error) {
	if uc.Tasks == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: tasks repository is nil")
	}
	if uc.EmbeddingPort == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: embedding port is nil")
	}
	if in.ProjectID == "" {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: project_id is required")
	}
	if in.TaskID <= 0 {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: task_id is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: task project mismatch")
	}
	if task.TaskType != run.MultimodalTaskTypeEmbedding {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: unsupported task_type=%s", task.TaskType)
	}

	if uc.BudgetGate != nil {
		bg, err := uc.BudgetGate.Handle(ctx, EvaluateV22BudgetGateInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task budget gate: %w", err)
		}
		if !bg.Allowed {
			return ExecuteV22EmbeddingTaskOutput{Task: bg.Task}, nil
		}
		task = bg.Task
	}

	if uc.PolicyGate != nil {
		pg, err := uc.PolicyGate.Handle(ctx, EvaluateV22PolicyGateInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
			Action:    "multimodal.task.execute",
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task policy gate: %w", err)
		}
		if !pg.Allowed {
			return ExecuteV22EmbeddingTaskOutput{Task: pg.Task}, nil
		}
		task = pg.Task
	}

	if uc.MarkRunning == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: mark running usecase is nil")
	}
	runningOut, err := uc.MarkRunning.Handle(ctx, MarkMultimodalTaskRunningInput{
		ProjectID: in.ProjectID,
		TaskID:    in.TaskID,
	})
	if err != nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task mark running: %w", err)
	}
	task = runningOut.Task

	execOut, err := uc.EmbeddingPort.ExecuteEmbedding(ctx, run.EmbeddingExecutionInput{
		Task:      task,
		Selection: run.EngineSelection{},
	})
	if err != nil {
		if uc.MarkFailedSoft == nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task failed soft usecase is nil after port error: %w", err)
		}
		failedOut, markErr := uc.MarkFailedSoft.Handle(ctx, MarkMultimodalTaskFailedSoftInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if markErr != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task port error=%v, mark failed soft error=%w", err, markErr)
		}
		return ExecuteV22EmbeddingTaskOutput{
			Task: failedOut.Task,
		}, nil
	}

	if uc.RegisterResult == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: register result usecase is nil")
	}
	resultOut, err := uc.RegisterResult.Handle(ctx, RegisterMultimodalResultInput{
		ProjectID:                 in.ProjectID,
		TraceID:                   task.TraceID,
		RunID:                     task.RunID,
		TaskID:                    task.ID,
		ResultType:                run.MultimodalResultTypeEmbeddingCandidates,
		OutputHash:                execOut.OutputHash,
		PayloadEvidenceAssetID:    execOut.PayloadEvidenceAssetID,
		ConfidenceEvidenceAssetID: execOut.ConfidenceEvidenceAssetID,
	})
	if err != nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task register result: %w", err)
	}

	if uc.AttachResultOutputs != nil && len(execOut.GeneratedOutputs) > 0 {
		var outs []run.AttachMultimodalResultOutputInput
		for _, g := range execOut.GeneratedOutputs {
			outs = append(outs, run.AttachMultimodalResultOutputInput{
				ProjectID:  in.ProjectID,
				ResultID:   resultOut.Result.ID,
				EvidenceID: g.EvidenceID,
				OutputRole: g.OutputRole,
				Seq:        g.Seq,
			})
		}
		_, err := uc.AttachResultOutputs.Handle(ctx, AttachMultimodalResultOutputsInput{
			ProjectID: in.ProjectID,
			ResultID:  resultOut.Result.ID,
			Outputs:   outs,
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task attach result outputs: %w", err)
		}
	}

	if uc.NormalizeResult == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: normalize result usecase is nil")
	}
	normOut, err := uc.NormalizeResult.Handle(ctx, NormalizeV22MultimodalResultInput{
		ProjectID:       in.ProjectID,
		ResultID:        resultOut.Result.ID,
		NormalizedKind:  run.NormalizedMultimodalResultKindSimilarityCandidates,
		SummaryText:     execOut.SummaryText,
		ConfidenceScore: execOut.ConfidenceScore,
		ReasonCode:      execOut.ReasonCode,
	})
	if err != nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task normalize result: %w", err)
	}

	if execOut.ReviewRequired {
		if uc.EnqueueReview == nil || uc.MarkReviewRequired == nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: review path usecases are nil")
		}
		_, err := uc.EnqueueReview.Handle(ctx, EnqueueV22MultimodalReviewInput{
			ProjectID:          in.ProjectID,
			NormalizedResultID: normOut.NormalizedResult.ID,
			ReasonCode:         execOut.ReasonCode,
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task enqueue review: %w", err)
		}
		reviewTaskOut, err := uc.MarkReviewRequired.Handle(ctx, MarkMultimodalTaskReviewRequiredInput{
			ProjectID: in.ProjectID,
			TaskID:    task.ID,
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task mark review required: %w", err)
		}
		return ExecuteV22EmbeddingTaskOutput{
			Task:             reviewTaskOut.Task,
			Result:           &resultOut.Result,
			NormalizedResult: &normOut.NormalizedResult,
		}, nil
	}

	if uc.DownstreamHandoff != nil && in.DestinationKind != "" {
		_, err := uc.DownstreamHandoff.Handle(ctx, CreateV22DownstreamHandoffInput{
			ProjectID:          in.ProjectID,
			NormalizedResultID: normOut.NormalizedResult.ID,
			DestinationKind:    in.DestinationKind,
			ReasonCode:         execOut.ReasonCode,
		})
		if err != nil {
			return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task downstream handoff: %w", err)
		}
	}

	if uc.MarkSucceeded == nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task: mark succeeded usecase is nil")
	}
	succeededOut, err := uc.MarkSucceeded.Handle(ctx, MarkMultimodalTaskSucceededInput{
		ProjectID: in.ProjectID,
		TaskID:    task.ID,
	})
	if err != nil {
		return ExecuteV22EmbeddingTaskOutput{}, fmt.Errorf("execute v22 embedding task mark succeeded: %w", err)
	}

	return ExecuteV22EmbeddingTaskOutput{
		Task:             succeededOut.Task,
		Result:           &resultOut.Result,
		NormalizedResult: &normOut.NormalizedResult,
	}, nil
}