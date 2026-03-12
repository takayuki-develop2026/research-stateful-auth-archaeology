package usecase

import (
	"context"
	"fmt"
	"sort"

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

	outputs := make([]run.AttachMultimodalResultOutputInput, len(in.Outputs))
	copy(outputs, in.Outputs)

	sort.Slice(outputs, func(i, j int) bool {
		if outputs[i].Seq != outputs[j].Seq {
			return outputs[i].Seq < outputs[j].Seq
		}
		if outputs[i].OutputRole != outputs[j].OutputRole {
			return outputs[i].OutputRole < outputs[j].OutputRole
		}
		return outputs[i].EvidenceID < outputs[j].EvidenceID
	})

	seen := map[string]struct{}{}
	created := make([]run.MultimodalResultOutput, 0, len(outputs))

	for idx, item := range outputs {
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

		// adapter 側が seq=0 を返してもここで補正する
		if item.Seq <= 0 {
			item.Seq = idx + 1
		}

		key := fmt.Sprintf("%d|%s|%d", item.EvidenceID, item.OutputRole, item.Seq)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

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