package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type AttachMultimodalTaskInputsUseCase struct {
	Tasks      run.MultimodalTaskRepository
	TaskInputs run.MultimodalTaskInputRepository
}

type AttachMultimodalTaskInputsInput struct {
	ProjectID string
	TaskID    int64
	Inputs    []run.AttachMultimodalTaskInputInput
}

type AttachMultimodalTaskInputsOutput struct {
	Task   run.MultimodalTask
	Inputs []run.MultimodalTaskInput
}

func (uc *AttachMultimodalTaskInputsUseCase) Handle(ctx context.Context, in AttachMultimodalTaskInputsInput) (AttachMultimodalTaskInputsOutput, error) {
	if uc.Tasks == nil {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: tasks repository is nil")
	}
	if uc.TaskInputs == nil {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: task inputs repository is nil")
	}
	if in.ProjectID == "" {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: project_id is required")
	}
	if in.TaskID <= 0 {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: task_id is required")
	}
	if len(in.Inputs) == 0 {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: inputs are required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: task project mismatch")
	}

	var created []run.MultimodalTaskInput
	for _, item := range in.Inputs {
		if item.ProjectID == "" {
			item.ProjectID = in.ProjectID
		}
		if item.TaskID == 0 {
			item.TaskID = in.TaskID
		}
		if item.ProjectID != in.ProjectID {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: input project mismatch")
		}
		if item.TaskID != in.TaskID {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: input task mismatch")
		}
		if item.EvidenceID <= 0 {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: evidence_id is required")
		}
		if item.InputRole == "" {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: input_role is required")
		}
		if item.Seq < 0 {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: seq must be >= 0")
		}

		v, err := uc.TaskInputs.Create(ctx, item)
		if err != nil {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs create: %w", err)
		}
		created = append(created, v)
	}

	return AttachMultimodalTaskInputsOutput{
		Task:   task,
		Inputs: created,
	}, nil
}
