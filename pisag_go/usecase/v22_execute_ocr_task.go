package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type ExecuteV22OCRTaskUseCase struct {
	Tasks              run.MultimodalTaskRepository
	BudgetGate         *EvaluateV22BudgetGateUseCase
	PolicyGate         *EvaluateV22PolicyGateUseCase
	MarkRunning        *MarkMultimodalTaskRunningUseCase
	MarkSucceeded      *MarkMultimodalTaskSucceededUseCase
	MarkReviewRequired *MarkMultimodalTaskReviewRequiredUseCase
	MarkFailedSoft     *MarkMultimodalTaskFailedSoftUseCase

	RegisterModelRun    *RegisterV22ModelRunUseCase
	RegisterOCREvidence *RegisterOCRResultEvidenceUseCase
	RegisterResult      *RegisterMultimodalResultUseCase
	AttachResultOutputs *AttachMultimodalResultOutputsUseCase
	NormalizeResult     *NormalizeV22MultimodalResultUseCase
	EnqueueReview       *EnqueueV22MultimodalReviewUseCase
	DownstreamHandoff   *CreateV22DownstreamHandoffUseCase

	OCRPort run.OCRExecutionPort
}

type ExecuteV22OCRTaskInput struct {
	ProjectID       string
	TaskID          int64
	DestinationKind string
}

type ExecuteV22OCRTaskOutput struct {
	Task             run.MultimodalTask
	Result           *run.MultimodalResult
	NormalizedResult *run.NormalizedMultimodalResult
}

func (uc *ExecuteV22OCRTaskUseCase) Handle(ctx context.Context, in ExecuteV22OCRTaskInput) (ExecuteV22OCRTaskOutput, error) {
	if uc.Tasks == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: tasks repository is nil")
	}
	if uc.OCRPort == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: ocr port is nil")
	}
	if in.ProjectID == "" {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: project_id is required")
	}
	if in.TaskID <= 0 {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: task_id is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: task project mismatch")
	}
	if task.TaskType != run.MultimodalTaskTypeOCR && task.TaskType != run.MultimodalTaskTypeFulltextExtract {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: unsupported task_type=%s", task.TaskType)
	}

	if uc.BudgetGate != nil {
		bg, err := uc.BudgetGate.Handle(ctx, EvaluateV22BudgetGateInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if err != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task budget gate: %w", err)
		}
		if !bg.Allowed {
			return ExecuteV22OCRTaskOutput{
				Task: bg.Task,
			}, nil
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
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task policy gate: %w", err)
		}
		if !pg.Allowed {
			return ExecuteV22OCRTaskOutput{
				Task: pg.Task,
			}, nil
		}
		task = pg.Task
	}

	if uc.MarkRunning == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: mark running usecase is nil")
	}
	runningOut, err := uc.MarkRunning.Handle(ctx, MarkMultimodalTaskRunningInput{
		ProjectID: in.ProjectID,
		TaskID:    in.TaskID,
	})
	if err != nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task mark running: %w", err)
	}
	task = runningOut.Task

	execOut, err := uc.OCRPort.ExecuteOCR(ctx, run.OCRExecutionInput{
		Task: task,
	})
	if err != nil {
		if uc.MarkFailedSoft == nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task failed soft usecase is nil after port error: %w", err)
		}
		failedOut, markErr := uc.MarkFailedSoft.Handle(ctx, MarkMultimodalTaskFailedSoftInput{
			ProjectID: in.ProjectID,
			TaskID:    in.TaskID,
		})
		if markErr != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task port error=%v, mark failed soft error=%w", err, markErr)
		}
		return ExecuteV22OCRTaskOutput{
			Task: failedOut.Task,
		}, nil
	}

	payloadEvidenceAssetID := execOut.PayloadEvidenceAssetID
	confidenceEvidenceAssetID := execOut.ConfidenceEvidenceAssetID
	generatedOutputs := execOut.GeneratedOutputs

	if uc.RegisterOCREvidence != nil {
		blocks := extractOCRBlocks(execOut.Metadata)

		evOut, err := uc.RegisterOCREvidence.Handle(ctx, RegisterOCRResultEvidenceInput{
			ProjectID:       in.ProjectID,
			TraceID:         task.TraceID,
			Text:            execOut.SummaryText,
			ConfidenceScore: execOut.ConfidenceScore,
			Blocks:          blocks,
			Metadata:        execOut.Metadata,
		})
		if err != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task register ocr evidence: %w", err)
		}

		payloadEvidenceAssetID = evOut.TextEvidenceAssetID
		confidenceEvidenceAssetID = &evOut.ConfidenceEvidenceAssetID

		if evOut.BlocksEvidenceAssetID != nil {
			generatedOutputs = append(generatedOutputs, run.MultimodalGeneratedOutput{
				EvidenceID: *evOut.BlocksEvidenceAssetID,
				OutputRole: run.MultimodalOutputRoleModelOutput,
				Seq:        len(generatedOutputs) + 1,
			})
		}
	}

	if uc.RegisterModelRun != nil {
		_, err := uc.RegisterModelRun.Handle(ctx, RegisterV22ModelRunInput{
			ProjectID:     in.ProjectID,
			TaskID:        task.ID,
			Capability:    "ocr",
			EngineKind:    execOut.EngineKind,
			EngineVersion: execOut.EngineVersion,
			Status:        "succeeded",
			Metadata:      execOut.Metadata,
		})
		if err != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task register model run: %w", err)
		}
	}

	if uc.RegisterResult == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: register result usecase is nil")
	}
	resultOut, err := uc.RegisterResult.Handle(ctx, RegisterMultimodalResultInput{
		ProjectID:                 in.ProjectID,
		TraceID:                   task.TraceID,
		RunID:                     task.RunID,
		TaskID:                    task.ID,
		ResultType:                run.MultimodalResultTypeOCRText,
		OutputHash:                execOut.OutputHash,
		PayloadEvidenceAssetID:    payloadEvidenceAssetID,
		ConfidenceEvidenceAssetID: confidenceEvidenceAssetID,
	})
	if err != nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task register result: %w", err)
	}

	if uc.AttachResultOutputs != nil && len(generatedOutputs) > 0 {
		var outs []run.AttachMultimodalResultOutputInput
		for _, g := range generatedOutputs {
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
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task attach result outputs: %w", err)
		}
	}

	if uc.NormalizeResult == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: normalize result usecase is nil")
	}
	normOut, err := uc.NormalizeResult.Handle(ctx, NormalizeV22MultimodalResultInput{
		ProjectID:       in.ProjectID,
		ResultID:        resultOut.Result.ID,
		NormalizedKind:  run.NormalizedMultimodalResultKindOCRText,
		SummaryText:     execOut.SummaryText,
		ConfidenceScore: execOut.ConfidenceScore,
		ReasonCode:      execOut.ReasonCode,
	})
	if err != nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task normalize result: %w", err)
	}

	if execOut.ReviewRequired {
		if uc.EnqueueReview == nil || uc.MarkReviewRequired == nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: review path usecases are nil")
		}
		_, err := uc.EnqueueReview.Handle(ctx, EnqueueV22MultimodalReviewInput{
			ProjectID:          in.ProjectID,
			NormalizedResultID: normOut.NormalizedResult.ID,
			ReasonCode:         execOut.ReasonCode,
		})
		if err != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task enqueue review: %w", err)
		}
		reviewTaskOut, err := uc.MarkReviewRequired.Handle(ctx, MarkMultimodalTaskReviewRequiredInput{
			ProjectID: in.ProjectID,
			TaskID:    task.ID,
		})
		if err != nil {
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task mark review required: %w", err)
		}
		return ExecuteV22OCRTaskOutput{
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
			return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task downstream handoff: %w", err)
		}
	}

	if uc.MarkSucceeded == nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task: mark succeeded usecase is nil")
	}
	succeededOut, err := uc.MarkSucceeded.Handle(ctx, MarkMultimodalTaskSucceededInput{
		ProjectID: in.ProjectID,
		TaskID:    task.ID,
	})
	if err != nil {
		return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task mark succeeded: %w", err)
	}

	return ExecuteV22OCRTaskOutput{
		Task:             succeededOut.Task,
		Result:           &resultOut.Result,
		NormalizedResult: &normOut.NormalizedResult,
	}, nil
}

func extractOCRBlocks(meta map[string]any) []map[string]any {
	if meta == nil {
		return nil
	}

	serviceMeta, ok := meta["service_meta"].(map[string]any)
	if !ok || serviceMeta == nil {
		return nil
	}

	raw, ok := serviceMeta["ocr_blocks"]
	if !ok {
		return nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil
	}

	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}