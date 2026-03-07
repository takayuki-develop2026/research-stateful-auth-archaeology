package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type CreateV22DownstreamHandoffUseCase struct {
	Downstream run.MultimodalDownstreamHandoffRepository
	Normalized run.NormalizedMultimodalResultRepository
}

type CreateV22DownstreamHandoffInput struct {
	ProjectID          string
	NormalizedResultID int64
	DestinationKind    string
	ReasonCode         string
}

type CreateV22DownstreamHandoffOutput struct {
	NormalizedResult run.NormalizedMultimodalResult
	Handoff          run.MultimodalDownstreamHandoff
}

func (uc *CreateV22DownstreamHandoffUseCase) Handle(ctx context.Context, in CreateV22DownstreamHandoffInput) (CreateV22DownstreamHandoffOutput, error) {
	if uc.Downstream == nil {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: downstream repository is nil")
	}
	if uc.Normalized == nil {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: normalized repository is nil")
	}
	if in.ProjectID == "" {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: project_id is required")
	}
	if in.NormalizedResultID <= 0 {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: normalized_result_id is required")
	}
	if in.DestinationKind == "" {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: destination_kind is required")
	}

	norm, err := uc.Normalized.FindByID(ctx, in.NormalizedResultID)
	if err != nil {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff load normalized result: %w", err)
	}
	if norm.ProjectID != in.ProjectID {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: normalized result project mismatch")
	}
	if norm.DownstreamPayloadEvidenceAssetID == nil || *norm.DownstreamPayloadEvidenceAssetID <= 0 {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff: downstream payload evidence is missing")
	}

	updatedNorm, err := uc.Normalized.UpdateStatus(ctx, in.ProjectID, norm.ID, run.NormalizedMultimodalResultStatusHandedOff)
	if err != nil {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff update normalized status: %w", err)
	}

	handoff, err := uc.Downstream.Create(ctx, run.CreateMultimodalDownstreamHandoffInput{
		ProjectID:              updatedNorm.ProjectID,
		TraceID:                updatedNorm.TraceID,
		RunID:                  updatedNorm.RunID,
		TaskID:                 updatedNorm.TaskID,
		ResultID:               updatedNorm.ResultID,
		NormalizedResultID:     updatedNorm.ID,
		DestinationKind:        in.DestinationKind,
		PayloadEvidenceAssetID: *updatedNorm.DownstreamPayloadEvidenceAssetID,
		HandoffStatus:          run.MultimodalDownstreamHandoffStatusPending,
		ReasonCode:             in.ReasonCode,
	})
	if err != nil {
		return CreateV22DownstreamHandoffOutput{}, fmt.Errorf("create v22 downstream handoff create: %w", err)
	}

	return CreateV22DownstreamHandoffOutput{
		NormalizedResult: updatedNorm,
		Handoff:          handoff,
	}, nil
}
