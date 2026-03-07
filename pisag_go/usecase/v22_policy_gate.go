package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type EvaluateV22PolicyGateUseCase struct {
	Tasks              run.MultimodalTaskRepository
	Gate               run.MultimodalPolicyGate
	MarkBlockedPolicy  *MarkMultimodalTaskBlockedPolicyUseCase
	MarkReviewRequired *MarkMultimodalTaskReviewRequiredUseCase
}

type EvaluateV22PolicyGateInput struct {
	ProjectID string
	TaskID    int64
	Action    string
}

type EvaluateV22PolicyGateOutput struct {
	Allowed  bool
	Decision run.MultimodalPolicyDecision
	Task     run.MultimodalTask
}

func (uc *EvaluateV22PolicyGateUseCase) Handle(ctx context.Context, in EvaluateV22PolicyGateInput) (EvaluateV22PolicyGateOutput, error) {
	if uc.Tasks == nil {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: tasks repository is nil")
	}
	if uc.Gate == nil {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: gate is nil")
	}
	if in.ProjectID == "" {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: project_id is required")
	}
	if in.TaskID <= 0 {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: task_id is required")
	}
	if in.Action == "" {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: action is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: task project mismatch")
	}

	decision, err := uc.Gate.EvaluatePolicy(ctx, run.EvaluateMultimodalPolicyInput{
		Task:   task,
		Action: in.Action,
	})
	if err != nil {
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate evaluate: %w", err)
	}

	switch decision.DecisionKind {
	case run.MultimodalPolicyDecisionAllow:
		return EvaluateV22PolicyGateOutput{
			Allowed:  true,
			Decision: decision,
			Task:     task,
		}, nil

	case run.MultimodalPolicyDecisionBlockedPolicy:
		if uc.MarkBlockedPolicy == nil {
			return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: mark blocked policy usecase is nil")
		}
		updated, err := uc.MarkBlockedPolicy.Handle(ctx, MarkMultimodalTaskBlockedPolicyInput{
			ProjectID:                in.ProjectID,
			TaskID:                   in.TaskID,
			SoftErrorEvidenceAssetID: decision.DetailEvidenceAssetID,
		})
		if err != nil {
			return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate mark blocked policy: %w", err)
		}
		return EvaluateV22PolicyGateOutput{
			Allowed:  false,
			Decision: decision,
			Task:     updated.Task,
		}, nil

	case run.MultimodalPolicyDecisionReviewRequired:
		if uc.MarkReviewRequired == nil {
			return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: mark review required usecase is nil")
		}
		updated, err := uc.MarkReviewRequired.Handle(ctx, MarkMultimodalTaskReviewRequiredInput{
			ProjectID:                in.ProjectID,
			TaskID:                   in.TaskID,
			SoftErrorEvidenceAssetID: decision.DetailEvidenceAssetID,
		})
		if err != nil {
			return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate mark review required: %w", err)
		}
		return EvaluateV22PolicyGateOutput{
			Allowed:  false,
			Decision: decision,
			Task:     updated.Task,
		}, nil

	default:
		return EvaluateV22PolicyGateOutput{}, fmt.Errorf("evaluate v22 policy gate: unsupported decision kind=%s", decision.DecisionKind)
	}
}
