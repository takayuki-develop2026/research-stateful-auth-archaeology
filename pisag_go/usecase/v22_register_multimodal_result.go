package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterMultimodalResultUseCase struct {
	Tasks   run.MultimodalTaskRepository
	Results run.MultimodalResultRepository
}

type RegisterMultimodalResultInput struct {
	ProjectID string
	TraceID   string
	RunID     string
	TaskID    int64

	ResultType                run.MultimodalResultType
	OutputHash                string
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
}

type RegisterMultimodalResultOutput struct {
	Task    run.MultimodalTask
	Result  run.MultimodalResult
	Created bool
}

func (uc *RegisterMultimodalResultUseCase) Handle(ctx context.Context, in RegisterMultimodalResultInput) (RegisterMultimodalResultOutput, error) {
	if uc.Tasks == nil {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: tasks repository is nil")
	}
	if uc.Results == nil {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: results repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: trace_id is required")
	}
	if strings.TrimSpace(in.RunID) == "" {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: run_id is required")
	}
	if in.TaskID <= 0 {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: task_id is required")
	}
	if in.ResultType == "" {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: result_type is required")
	}
	if strings.TrimSpace(in.OutputHash) == "" {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: output_hash is required")
	}
	if in.PayloadEvidenceAssetID <= 0 {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: payload_evidence_asset_id is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: task project mismatch")
	}
	if task.TraceID != in.TraceID {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: task trace mismatch")
	}
	if task.RunID != in.RunID {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result: task run mismatch")
	}

	resultKey := buildMultimodalResultKey(run.BuildMultimodalResultKeyInput{
		ProjectID:  in.ProjectID,
		TaskID:     in.TaskID,
		ResultType: in.ResultType,
		ModelRunID: task.ModelRunID,
		OutputHash: in.OutputHash,
	})

	existing, err := uc.Results.FindByProjectAndResultKey(ctx, in.ProjectID, resultKey)
	if err == nil {
		return RegisterMultimodalResultOutput{
			Task:    task,
			Result:  existing,
			Created: false,
		}, nil
	}

	result, err := uc.Results.Create(ctx, run.RegisterMultimodalResultInput{
		ProjectID:                 in.ProjectID,
		TraceID:                   in.TraceID,
		RunID:                     in.RunID,
		TaskID:                    in.TaskID,
		ResultKey:                 resultKey,
		ResultType:                in.ResultType,
		OutputHash:                in.OutputHash,
		PayloadEvidenceAssetID:    in.PayloadEvidenceAssetID,
		ConfidenceEvidenceAssetID: in.ConfidenceEvidenceAssetID,
	})
	if err != nil {
		return RegisterMultimodalResultOutput{}, fmt.Errorf("register multimodal result create: %w", err)
	}

	return RegisterMultimodalResultOutput{
		Task:    task,
		Result:  result,
		Created: true,
	}, nil
}

func buildMultimodalResultKey(in run.BuildMultimodalResultKeyInput) string {
	modelRun := ""
	if in.ModelRunID != nil {
		modelRun = strconv.FormatInt(*in.ModelRunID, 10)
	}

	s := strings.Join([]string{
		strings.TrimSpace(in.ProjectID),
		strconv.FormatInt(in.TaskID, 10),
		string(in.ResultType),
		modelRun,
		strings.TrimSpace(in.OutputHash),
	}, "|")

	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}