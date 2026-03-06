package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type AttachMultimodalResultOutputsUseCase struct {
	Results       run.MultimodalResultRepository
	ResultOutputs run.MultimodalResultOutputRepository
}

type AttachMultimodalResultOutputsInput struct {
	ProjectID string
	ResultID  int64
	Outputs   []run.AttachMultimodalResultOutputInput
}

type AttachMultimodalResultOutputsOutput struct {
	Result  run.MultimodalResult
	Outputs []run.MultimodalResultOutput
}

func (uc *AttachMultimodalResultOutputsUseCase) Handle(ctx context.Context, in AttachMultimodalResultOutputsInput) (AttachMultimodalResultOutputsOutput, error) {
	if uc.Results == nil {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: results repository is nil")
	}
	if uc.ResultOutputs == nil {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: result outputs repository is nil")
	}
	if in.ProjectID == "" {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: project_id is required")
	}
	if in.ResultID <= 0 {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: result_id is required")
	}
	if len(in.Outputs) == 0 {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: outputs are required")
	}

	result, err := uc.Results.FindByID(ctx, in.ResultID)
	if err != nil {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs load result: %w", err)
	}
	if result.ProjectID != in.ProjectID {
		return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: result project mismatch")
	}

	var created []run.MultimodalResultOutput
	for _, item := range in.Outputs {
		if item.ProjectID == "" {
			item.ProjectID = in.ProjectID
		}
		if item.ResultID == 0 {
			item.ResultID = in.ResultID
		}
		if item.ProjectID != in.ProjectID {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: output project mismatch")
		}
		if item.ResultID != in.ResultID {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: output result mismatch")
		}
		if item.EvidenceID <= 0 {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: evidence_id is required")
		}
		if item.OutputRole == "" {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: output_role is required")
		}
		if item.Seq < 0 {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs: seq must be >= 0")
		}

		v, err := uc.ResultOutputs.Create(ctx, item)
		if err != nil {
			return AttachMultimodalResultOutputsOutput{}, fmt.Errorf("attach multimodal result outputs create: %w", err)
		}
		created = append(created, v)
	}

	return AttachMultimodalResultOutputsOutput{
		Result:  result,
		Outputs: created,
	}, nil
}
