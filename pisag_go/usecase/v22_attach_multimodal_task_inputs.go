package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"

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

	inputs := make([]run.AttachMultimodalTaskInputInput, len(in.Inputs))
	copy(inputs, in.Inputs)

	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Seq != inputs[j].Seq {
			return inputs[i].Seq < inputs[j].Seq
		}
		if inputs[i].InputRole != inputs[j].InputRole {
			return inputs[i].InputRole < inputs[j].InputRole
		}
		return inputs[i].EvidenceID < inputs[j].EvidenceID
	})

	seen := map[string]struct{}{}
	created := make([]run.MultimodalTaskInput, 0, len(inputs))

	for _, item := range inputs {
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
		if item.Seq <= 0 {
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs: seq must be >= 1")
		}

		key := fmt.Sprintf("%d|%s|%d", item.EvidenceID, item.InputRole, item.Seq)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		v, err := uc.TaskInputs.Create(ctx, item)
		if err != nil {
			if isUniqueViolation(err) {
				// 既に同じ input が attach 済みなら成功扱いでスキップ
				continue
			}
			return AttachMultimodalTaskInputsOutput{}, fmt.Errorf("attach multimodal task inputs create: %w", err)
		}
		created = append(created, v)
	}

	return AttachMultimodalTaskInputsOutput{
		Task:   task,
		Inputs: created,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}