package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type EvaluateV22BudgetGateUseCase struct {
	Tasks              run.MultimodalTaskRepository
	Gate               run.MultimodalBudgetGate
	MarkSkippedBudget  *MarkMultimodalTaskSkippedBudgetUseCase
	MarkReviewRequired *MarkMultimodalTaskReviewRequiredUseCase
}

type EvaluateV22BudgetGateInput struct {
	ProjectID string
	TaskID    int64
}

type EvaluateV22BudgetGateOutput struct {
	Allowed  bool
	Decision run.MultimodalBudgetDecision
	Task     run.MultimodalTask
}

func (uc *EvaluateV22BudgetGateUseCase) Handle(ctx context.Context, in EvaluateV22BudgetGateInput) (EvaluateV22BudgetGateOutput, error) {
	if uc.Tasks == nil {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: tasks repository is nil")
	}
	if uc.Gate == nil {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: gate is nil")
	}
	if in.ProjectID == "" {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: project_id is required")
	}
	if in.TaskID <= 0 {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: task_id is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: task project mismatch")
	}

	decision, err := uc.Gate.EvaluateBudget(ctx, run.EvaluateMultimodalBudgetInput{
		Task: task,
	})
	if err != nil {
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate evaluate: %w", err)
	}

	switch decision.DecisionKind {
	case run.MultimodalBudgetDecisionAllow:
		return EvaluateV22BudgetGateOutput{
			Allowed:  true,
			Decision: decision,
			Task:     task,
		}, nil

	case run.MultimodalBudgetDecisionSkippedBudget:
		if uc.MarkSkippedBudget == nil {
			return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: mark skipped budget usecase is nil")
		}
		updated, err := uc.MarkSkippedBudget.Handle(ctx, MarkMultimodalTaskSkippedBudgetInput{
			ProjectID:                in.ProjectID,
			TaskID:                   in.TaskID,
			SoftErrorEvidenceAssetID: decision.DetailEvidenceAssetID,
		})
		if err != nil {
			return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate mark skipped budget: %w", err)
		}
		return EvaluateV22BudgetGateOutput{
			Allowed:  false,
			Decision: decision,
			Task:     updated.Task,
		}, nil

	case run.MultimodalBudgetDecisionReviewRequired:
		if uc.MarkReviewRequired == nil {
			return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: mark review required usecase is nil")
		}
		updated, err := uc.MarkReviewRequired.Handle(ctx, MarkMultimodalTaskReviewRequiredInput{
			ProjectID:                in.ProjectID,
			TaskID:                   in.TaskID,
			SoftErrorEvidenceAssetID: decision.DetailEvidenceAssetID,
		})
		if err != nil {
			return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate mark review required: %w", err)
		}
		return EvaluateV22BudgetGateOutput{
			Allowed:  false,
			Decision: decision,
			Task:     updated.Task,
		}, nil

	default:
		return EvaluateV22BudgetGateOutput{}, fmt.Errorf("evaluate v22 budget gate: unsupported decision kind=%s", decision.DecisionKind)
	}
}
