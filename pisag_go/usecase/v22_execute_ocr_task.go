package usecase

import (
	"context"
	"fmt"
	"strings"

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

	PreprocessPort run.PreprocessExecutionPort
	OCRPort        run.OCRExecutionPort
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
	if strings.TrimSpace(in.ProjectID) == "" {
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

	// 現在の run.MultimodalTask に EngineSelectionJSON が無い前提の安全版。
	// 後で run 側に engine_selection_json を正式追加したら差し替える。
	selection := run.EngineSelection{}

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

	// preprocess は実行する。
	// ただし現行 run.OCRExecutionInput に SourceEvidenceID が無いので、
	// preprocess 出力の受け渡しは adapter 側の内部解決に寄せる。
	if uc.PreprocessPort != nil && len(selection.Preprocess) > 0 {
		preOut, err := uc.PreprocessPort.ExecutePreprocess(ctx, run.PreprocessExecutionInput{
			Task:             task,
			Selection:        selection,
			SourceEvidenceID: nil,
		})
		if err != nil {
			if uc.MarkFailedSoft == nil {
				return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task preprocess failed and failed_soft usecase is nil: %w", err)
			}
			failedOut, markErr := uc.MarkFailedSoft.Handle(ctx, MarkMultimodalTaskFailedSoftInput{
				ProjectID: in.ProjectID,
				TaskID:    in.TaskID,
			})
			if markErr != nil {
				return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task preprocess error=%v, mark failed soft error=%w", err, markErr)
			}
			return ExecuteV22OCRTaskOutput{
				Task: failedOut.Task,
			}, nil
		}

		if uc.RegisterModelRun != nil {
			_, err := uc.RegisterModelRun.Handle(ctx, RegisterV22ModelRunInput{
				ProjectID:     in.ProjectID,
				TaskID:        task.ID,
				Capability:    "preprocess",
				EngineKind:    preOut.EngineKind,
				EngineVersion: preOut.EngineVersion,
				Status:        "succeeded",
				Metadata:      preOut.Metadata,
			})
			if err != nil {
				return ExecuteV22OCRTaskOutput{}, fmt.Errorf("execute v22 ocr task register preprocess model run: %w", err)
			}
		}
	}

	execOut, err := uc.OCRPort.ExecuteOCR(ctx, run.OCRExecutionInput{
		Task:      task,
		Selection: selection,
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
			FullText:        extractOCRFullText(execOut.Metadata, execOut.SummaryText),
			SummaryText:     strings.TrimSpace(execOut.SummaryText),
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
		TaskID:    in.TaskID,
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

func extractOCRFullText(meta map[string]any, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if meta == nil {
		return fallback
	}

	serviceMeta, ok := meta["service_meta"].(map[string]any)
	if ok && serviceMeta != nil {
		if v, ok := serviceMeta["ocr_text"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := serviceMeta["text"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	if v, ok := meta["ocr_text"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := meta["text"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}

	return fallback
}