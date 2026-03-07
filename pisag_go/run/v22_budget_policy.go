package run

import "context"

type MultimodalBudgetDecisionKind string

const (
	MultimodalBudgetDecisionAllow          MultimodalBudgetDecisionKind = "allow"
	MultimodalBudgetDecisionSkippedBudget  MultimodalBudgetDecisionKind = "skipped_budget"
	MultimodalBudgetDecisionReviewRequired MultimodalBudgetDecisionKind = "review_required"
)

type MultimodalPolicyDecisionKind string

const (
	MultimodalPolicyDecisionAllow          MultimodalPolicyDecisionKind = "allow"
	MultimodalPolicyDecisionBlockedPolicy  MultimodalPolicyDecisionKind = "blocked_policy"
	MultimodalPolicyDecisionReviewRequired MultimodalPolicyDecisionKind = "review_required"
)

type MultimodalBudgetDecision struct {
	DecisionKind          MultimodalBudgetDecisionKind
	ReasonCode            string
	DetailEvidenceAssetID *int64
}

type MultimodalPolicyDecision struct {
	DecisionKind          MultimodalPolicyDecisionKind
	ReasonCode            string
	DetailEvidenceAssetID *int64
}

type EvaluateMultimodalBudgetInput struct {
	Task MultimodalTask
}

type EvaluateMultimodalPolicyInput struct {
	Task   MultimodalTask
	Action string
}

type MultimodalBudgetGate interface {
	EvaluateBudget(ctx context.Context, in EvaluateMultimodalBudgetInput) (MultimodalBudgetDecision, error)
}

type MultimodalPolicyGate interface {
	EvaluatePolicy(ctx context.Context, in EvaluateMultimodalPolicyInput) (MultimodalPolicyDecision, error)
}
