package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterPIIRedactionUseCase struct {
	Redactions run.PIIRedactionRepository
}

type RegisterPIIRedactionInput struct {
	ProjectID             string
	TraceID               string
	EvidenceID            int64
	PolicyDecisionID      int64
	RuleKey               string
	Action                run.PIIRedactionAction
	AppliedByType         run.PIIRedactionAppliedByType
	AppliedByID           string
	DetailEvidenceAssetID int64
}

type RegisterPIIRedactionOutput struct {
	Redaction run.PIIRedaction
}

func (uc *RegisterPIIRedactionUseCase) Handle(ctx context.Context, in RegisterPIIRedactionInput) (RegisterPIIRedactionOutput, error) {
	if uc.Redactions == nil {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: redactions repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: trace_id is required")
	}
	if in.EvidenceID <= 0 {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: evidence_id is required")
	}
	if in.PolicyDecisionID <= 0 {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: policy_decision_id is required")
	}
	if strings.TrimSpace(in.RuleKey) == "" {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: rule_key is required")
	}
	if in.Action == "" {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: action is required")
	}
	if in.AppliedByType == "" {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: applied_by_type is required")
	}
	if in.DetailEvidenceAssetID <= 0 {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction: detail_evidence_asset_id is required")
	}

	redaction, err := uc.Redactions.Create(ctx, run.RegisterPIIRedactionInput{
		ProjectID:             in.ProjectID,
		TraceID:               in.TraceID,
		EvidenceID:            in.EvidenceID,
		PolicyDecisionID:      in.PolicyDecisionID,
		RuleKey:               strings.TrimSpace(in.RuleKey),
		Action:                in.Action,
		AppliedByType:         in.AppliedByType,
		AppliedByID:           strings.TrimSpace(in.AppliedByID),
		DetailEvidenceAssetID: in.DetailEvidenceAssetID,
	})
	if err != nil {
		return RegisterPIIRedactionOutput{}, fmt.Errorf("register pii redaction create: %w", err)
	}

	return RegisterPIIRedactionOutput{
		Redaction: redaction,
	}, nil
}
