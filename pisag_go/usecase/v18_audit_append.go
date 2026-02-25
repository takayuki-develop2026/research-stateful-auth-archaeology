package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type AppendAuditV18UseCase struct {
	AuditRepo run.AuditV18Repo
}

type AppendAuditV18Input struct {
	ProjectID string
	TraceID   string
	RunID     *string

	Action string

	ActorType string
	ActorID   *string

	TargetType *string
	TargetID   *string

	Result string
	Reason *string

	Meta map[string]any

	IdempotencyKey *string
}

func (uc *AppendAuditV18UseCase) Handle(ctx context.Context, in AppendAuditV18Input) (run.AuditAppendResult, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return run.AuditAppendResult{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return run.AuditAppendResult{}, errors.New("trace_id is required")
	}
	if strings.TrimSpace(in.Action) == "" {
		return run.AuditAppendResult{}, errors.New("action is required")
	}
	if strings.TrimSpace(in.ActorType) == "" {
		return run.AuditAppendResult{}, errors.New("actor_type is required")
	}
	if strings.TrimSpace(in.Result) == "" {
		return run.AuditAppendResult{}, errors.New("result is required")
	}
	if uc.AuditRepo == nil {
		return run.AuditAppendResult{}, errors.New("AuditRepo is required")
	}

	var metaJSON []byte
	if in.Meta != nil {
		b, err := json.Marshal(in.Meta)
		if err == nil && len(b) > 0 {
			metaJSON = b
		}
	}

	return uc.AuditRepo.Append(ctx, run.AuditAppendInput{
		ProjectID:      in.ProjectID,
		TraceID:        in.TraceID,
		RunID:          in.RunID,
		Action:         in.Action,
		ActorType:      in.ActorType,
		ActorID:        in.ActorID,
		TargetType:     in.TargetType,
		TargetID:       in.TargetID,
		Result:         in.Result,
		Reason:         in.Reason,
		MetaJSON:       metaJSON,
		IdempotencyKey: in.IdempotencyKey,
	})
}