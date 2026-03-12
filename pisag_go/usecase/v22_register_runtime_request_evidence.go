package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterRuntimeRequestEvidencePort interface {
	RegisterJSONEvidence(ctx context.Context, in run.RegisterRuntimeJSONEvidenceInput) (run.RegisterRuntimeJSONEvidenceOutput, error)
}

type RegisterRuntimeRequestEvidenceUseCase struct {
	Evidence RegisterRuntimeRequestEvidencePort
}

type RegisterRuntimeRequestEvidenceInput struct {
	ProjectID               string
	TraceID                 string
	OptionsCanonicalJSON    string
	RoutePlanCanonicalJSON  string
	OptionsSHA256           string
	RoutePlanSHA256         string
}

type RegisterRuntimeRequestEvidenceOutput struct {
	OptionsEvidenceAssetID    int64
	RouterPlanEvidenceAssetID int64
}

func (uc *RegisterRuntimeRequestEvidenceUseCase) Handle(ctx context.Context, in RegisterRuntimeRequestEvidenceInput) (RegisterRuntimeRequestEvidenceOutput, error) {
	if uc.Evidence == nil {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: evidence port is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: trace_id is required")
	}
	if strings.TrimSpace(in.OptionsCanonicalJSON) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: options_canonical_json is required")
	}
	if strings.TrimSpace(in.RoutePlanCanonicalJSON) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: route_plan_canonical_json is required")
	}
	if strings.TrimSpace(in.OptionsSHA256) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: options_sha256 is required")
	}
	if strings.TrimSpace(in.RoutePlanSHA256) == "" {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence: route_plan_sha256 is required")
	}

	optionsOut, err := uc.Evidence.RegisterJSONEvidence(ctx, run.RegisterRuntimeJSONEvidenceInput{
		ProjectID:   in.ProjectID,
		TraceID:     in.TraceID,
		Kind:        "runtime_options",
		BodyJSON:    in.OptionsCanonicalJSON,
		SHA256:      in.OptionsSHA256,
		Description: "AI Runtime engine selection canonical options",
	})
	if err != nil {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence options: %w", err)
	}

	routeOut, err := uc.Evidence.RegisterJSONEvidence(ctx, run.RegisterRuntimeJSONEvidenceInput{
		ProjectID:   in.ProjectID,
		TraceID:     in.TraceID,
		Kind:        "runtime_route_plan",
		BodyJSON:    in.RoutePlanCanonicalJSON,
		SHA256:      in.RoutePlanSHA256,
		Description: "AI Runtime route plan canonical json",
	})
	if err != nil {
		return RegisterRuntimeRequestEvidenceOutput{}, fmt.Errorf("register runtime request evidence route plan: %w", err)
	}

	return RegisterRuntimeRequestEvidenceOutput{
		OptionsEvidenceAssetID:    optionsOut.EvidenceAssetID,
		RouterPlanEvidenceAssetID: routeOut.EvidenceAssetID,
	}, nil
}