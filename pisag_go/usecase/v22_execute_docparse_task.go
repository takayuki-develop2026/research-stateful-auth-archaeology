package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type ExecuteV22DocParseTaskUseCase struct {
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

	DocParsePort run.DocParseExecutionPort
}

type ExecuteV22DocParseTaskInput struct {
	ProjectID       string
	TaskID          int64
	DestinationKind string
}

type ExecuteV22DocParseTaskOutput struct {
	Task             run.MultimodalTask
	Result           *run.MultimodalResult
	NormalizedResult *run.NormalizedMultimodalResult
}

func (uc *ExecuteV22DocParseTaskUseCase) Handle(ctx context.Context, in ExecuteV22DocParseTaskInput) (ExecuteV22DocParseTaskOutput, error) {
	if uc.Tasks == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: tasks repository is nil")
	}
	if uc.DocParsePort == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: docparse port is nil")
	}
	if in.ProjectID == "" {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: project_id is required")
	}
	if in.TaskID <= 0 {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: task_id is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: task project mismatch")
	}
	if task.TaskType != run.MultimodalTaskTypeDocParse {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: unsupported task_type=%s", task.TaskType)
	}

	if uc.BudgetGate != nil {
		bg, err := uc.BudgetGate.Handle(ctx, EvaluateV22BudgetGateInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if err != nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task budget gate: %w", err)
		}
		if !bg.Allowed {
			return ExecuteV22DocParseTaskOutput{Task: bg.Task}, nil
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
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task policy gate: %w", err)
		}
		if !pg.Allowed {
			return ExecuteV22DocParseTaskOutput{Task: pg.Task}, nil
		}
		task = pg.Task
	}

	if uc.MarkRunning == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: mark running usecase is nil")
	}
	runningOut, err := uc.MarkRunning.Handle(ctx, MarkMultimodalTaskRunningInput{
		ProjectID: in.ProjectID,
		TaskID:    in.TaskID,
	})
	if err != nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task mark running: %w", err)
	}
	task = runningOut.Task

	execOut, err := uc.DocParsePort.ExecuteDocParse(ctx, run.DocParseExecutionInput{
		Task:      task,
		Selection: run.EngineSelection{},
	})
	if err != nil {
		if uc.MarkFailedSoft == nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task failed soft usecase is nil after port error: %w", err)
		}
		failedOut, markErr := uc.MarkFailedSoft.Handle(ctx, MarkMultimodalTaskFailedSoftInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if markErr != nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task port error=%v, mark failed soft error=%w", err, markErr)
		}
		return ExecuteV22DocParseTaskOutput{
			Task: failedOut.Task,
		}, nil
	}

	if uc.RegisterResult == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: register result usecase is nil")
	}
	resultOut, err := uc.RegisterResult.Handle(ctx, RegisterMultimodalResultInput{
		ProjectID:                 in.ProjectID,
		TraceID:                   task.TraceID,
		RunID:                     task.RunID,
		TaskID:                    task.ID,
		ResultType:                run.MultimodalResultTypeDocParseStructure,
		OutputHash:                execOut.OutputHash,
		PayloadEvidenceAssetID:    execOut.PayloadEvidenceAssetID,
		ConfidenceEvidenceAssetID: execOut.ConfidenceEvidenceAssetID,
	})
	if err != nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task register result: %w", err)
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
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task attach result outputs: %w", err)
		}
	}

	if uc.NormalizeResult == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: normalize result usecase is nil")
	}
	normOut, err := uc.NormalizeResult.Handle(ctx, NormalizeV22MultimodalResultInput{
		ProjectID:       in.ProjectID,
		ResultID:        resultOut.Result.ID,
		NormalizedKind:  run.NormalizedMultimodalResultKindDocumentStructure,
		SummaryText:     execOut.SummaryText,
		ConfidenceScore: execOut.ConfidenceScore,
		ReasonCode:      execOut.ReasonCode,
	})
	if err != nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task normalize result: %w", err)
	}

	if execOut.ReviewRequired {
		if uc.EnqueueReview == nil || uc.MarkReviewRequired == nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: review path usecases are nil")
		}
		_, err := uc.EnqueueReview.Handle(ctx, EnqueueV22MultimodalReviewInput{
			ProjectID:          in.ProjectID,
			NormalizedResultID: normOut.NormalizedResult.ID,
			ReasonCode:         execOut.ReasonCode,
		})
		if err != nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task enqueue review: %w", err)
		}
		reviewTaskOut, err := uc.MarkReviewRequired.Handle(ctx, MarkMultimodalTaskReviewRequiredInput{
			ProjectID: in.ProjectID,
			TaskID:    task.ID,
		})
		if err != nil {
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task mark review required: %w", err)
		}
		return ExecuteV22DocParseTaskOutput{
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
			return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task downstream handoff: %w", err)
		}
	}

	if uc.MarkSucceeded == nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task: mark succeeded usecase is nil")
	}
	succeededOut, err := uc.MarkSucceeded.Handle(ctx, MarkMultimodalTaskSucceededInput{
		ProjectID: in.ProjectID,
		TaskID:    task.ID,
	})
	if err != nil {
		return ExecuteV22DocParseTaskOutput{}, fmt.Errorf("execute v22 docparse task mark succeeded: %w", err)
	}

	return ExecuteV22DocParseTaskOutput{
		Task:             succeededOut.Task,
		Result:           &resultOut.Result,
		NormalizedResult: &normOut.NormalizedResult,
	}, nil
}