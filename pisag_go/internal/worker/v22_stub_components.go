package worker

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type ConventionBudgetGate struct{}

func (g *ConventionBudgetGate) EvaluateBudget(_ context.Context, in run.EvaluateMultimodalBudgetInput) (run.MultimodalBudgetDecision, error) {
	if strings.Contains(in.Task.PipelineVersion, "budget-skip") {
		id := in.Task.OptionsEvidenceAssetID
		return run.MultimodalBudgetDecision{
			DecisionKind:          run.MultimodalBudgetDecisionSkippedBudget,
			ReasonCode:            "budget_insufficient",
			DetailEvidenceAssetID: &id,
		}, nil
	}
	if strings.Contains(in.Task.PipelineVersion, "budget-review") {
		id := in.Task.OptionsEvidenceAssetID
		return run.MultimodalBudgetDecision{
			DecisionKind:          run.MultimodalBudgetDecisionReviewRequired,
			ReasonCode:            "budget_review_required",
			DetailEvidenceAssetID: &id,
		}, nil
	}
	return run.MultimodalBudgetDecision{
		DecisionKind: run.MultimodalBudgetDecisionAllow,
		ReasonCode:   "allowed",
	}, nil
}

type ConventionPolicyGate struct{}

func (g *ConventionPolicyGate) EvaluatePolicy(_ context.Context, in run.EvaluateMultimodalPolicyInput) (run.MultimodalPolicyDecision, error) {
	if strings.Contains(in.Task.PolicyVersionStr, "policy-block") {
		id := in.Task.OptionsEvidenceAssetID
		return run.MultimodalPolicyDecision{
			DecisionKind:          run.MultimodalPolicyDecisionBlockedPolicy,
			ReasonCode:            "policy_blocked",
			DetailEvidenceAssetID: &id,
		}, nil
	}
	if strings.Contains(in.Task.PolicyVersionStr, "policy-review") {
		id := in.Task.OptionsEvidenceAssetID
		return run.MultimodalPolicyDecision{
			DecisionKind:          run.MultimodalPolicyDecisionReviewRequired,
			ReasonCode:            "policy_review_required",
			DetailEvidenceAssetID: &id,
		}, nil
	}
	return run.MultimodalPolicyDecision{
		DecisionKind: run.MultimodalPolicyDecisionAllow,
		ReasonCode:   "allowed",
	}, nil
}

type StubOCRAdapter struct{}

func (a *StubOCRAdapter) ExecuteOCR(_ context.Context, in run.OCRExecutionInput) (run.OCRExecutionOutput, error) {
	if strings.Contains(in.Task.PipelineVersion, "ocr-fail") {
		return run.OCRExecutionOutput{}, fmt.Errorf("stub ocr adapter forced failure")
	}

	conf := 0.93
	reviewRequired := strings.Contains(in.Task.PolicyVersionStr, "ocr-review")
	if reviewRequired {
		conf = 0.42
	}

	return run.OCRExecutionOutput{
		PayloadEvidenceAssetID:    in.Task.OptionsEvidenceAssetID,
		ConfidenceEvidenceAssetID: int64Ptr(in.Task.OptionsEvidenceAssetID),
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: in.Task.RouterPlanEvidenceAssetID,
				OutputRole: run.MultimodalOutputRoleAnnotatedImage,
				Seq:        1,
			},
		},
		OutputHash:      fmt.Sprintf("stub_ocr_output_%d", in.Task.ID),
		SummaryText:     "stub ocr extracted text",
		ConfidenceScore: &conf,
		ReasonCode:      reasonCode(reviewRequired, "ocr_low_confidence", "ocr_ok"),
		ReviewRequired:  reviewRequired,
		EngineKind:      run.EngineKindPaddleOCR,
		EngineVersion:   "v1",
		Metadata: map[string]any{
			"adapter":    "stub_ocr",
			"project_id": in.Task.ProjectID,
			"task_id":    in.Task.ID,
			"task_type":  string(in.Task.TaskType),
		},
	}, nil
}

type StubVisionAdapter struct{}

func (a *StubVisionAdapter) ExecuteVision(_ context.Context, in run.VisionExecutionInput) (run.VisionExecutionOutput, error) {
	if strings.Contains(in.Task.PipelineVersion, "vision-fail") {
		return run.VisionExecutionOutput{}, fmt.Errorf("stub vision adapter forced failure")
	}

	conf := 0.91
	reviewRequired := strings.Contains(in.Task.PolicyVersionStr, "vision-review")
	if reviewRequired {
		conf = 0.39
	}

	return run.VisionExecutionOutput{
		PayloadEvidenceAssetID:    in.Task.OptionsEvidenceAssetID,
		ConfidenceEvidenceAssetID: int64Ptr(in.Task.OptionsEvidenceAssetID),
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: in.Task.RouterPlanEvidenceAssetID,
				OutputRole: run.MultimodalOutputRoleAnnotatedImage,
				Seq:        1,
			},
		},
		OutputHash:      fmt.Sprintf("stub_vision_output_%d", in.Task.ID),
		SummaryText:     "stub vision detected entities",
		ConfidenceScore: &conf,
		ReasonCode:      reasonCode(reviewRequired, "vision_low_confidence", "vision_ok"),
		ReviewRequired:  reviewRequired,
		EngineKind:      run.EngineKindQwenVL,
		EngineVersion:   "v1",
		Metadata: map[string]any{
			"adapter":    "stub_vision",
			"project_id": in.Task.ProjectID,
			"task_id":    in.Task.ID,
			"task_type":  string(in.Task.TaskType),
		},
	}, nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

func reasonCode(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}