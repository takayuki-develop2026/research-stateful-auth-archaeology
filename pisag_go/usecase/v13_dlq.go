package usecase

import (
	"context"
	"errors"
	"strings"

	"example.com/pisag_go/postgres"
)

type V13DlqEnqueueInput struct {
	ProjectID      string
	RunID          *string
	TraceID        string
	TaskType       string
	Source         string
	CorrelationKey *string

	PayloadEvidenceAssetID   int64
	LastErrorEvidenceAssetID *int64
}

type V13DlqEnqueueUseCase struct {
	V13Repo *postgres.V13Repository
}

func (uc *V13DlqEnqueueUseCase) Handle(ctx context.Context, in V13DlqEnqueueInput) (int64, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return 0, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return 0, errors.New("trace_id is required")
	}
	if strings.TrimSpace(in.TaskType) == "" {
		return 0, errors.New("task_type is required")
	}
	if strings.TrimSpace(in.Source) == "" {
		return 0, errors.New("source is required")
	}
	if in.PayloadEvidenceAssetID <= 0 {
		return 0, errors.New("payload_evidence_asset_id is required")
	}
	return uc.V13Repo.DlqEnqueue(ctx,
		strings.TrimSpace(in.ProjectID),
		in.RunID,
		strings.TrimSpace(in.TraceID),
		strings.TrimSpace(in.TaskType),
		strings.TrimSpace(in.Source),
		in.CorrelationKey,
		in.PayloadEvidenceAssetID,
		in.LastErrorEvidenceAssetID,
	)
}

type V13DlqMarkInput struct {
	ProjectID                  string
	DlqID                      int64
	Status                     string // requeued|resolved|ignored
	ResultErrorEvidenceAssetID *int64
}

type V13DlqMarkUseCase struct {
	V13Repo *postgres.V13Repository
}

func (uc *V13DlqMarkUseCase) Handle(ctx context.Context, in V13DlqMarkInput) error {
	if strings.TrimSpace(in.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if in.DlqID <= 0 {
		return errors.New("dlq_id is required")
	}
	if strings.TrimSpace(in.Status) == "" {
		return errors.New("status is required")
	}
	return uc.V13Repo.DlqMark(ctx, strings.TrimSpace(in.ProjectID), in.DlqID, strings.TrimSpace(in.Status), in.ResultErrorEvidenceAssetID)
}
