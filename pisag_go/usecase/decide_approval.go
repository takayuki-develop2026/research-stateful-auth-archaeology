package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

// DecideApprovalInput appends a decision to approval_decisions and updates request status.
// v4.7 default-deny: publish can be confirmed only after approved.
type DecideApprovalInput struct {
	ProjectID string
	RequestID string

	TraceID string

	Decision string // "approve" | "reject"

	DecidedByType string  // "system" | "user" (default user)
	DecidedByID   *string // nullable
	Comment       *string // nullable
}

type DecideApprovalOutput struct {
	ProjectID string
	RequestID string
	TraceID   string

	Decision string
	Status   string // pending/approved/rejected
}

type DecideApprovalUseCase struct {
	ApprovalRepo run.ApprovalRepo
}

func (uc *DecideApprovalUseCase) Handle(ctx context.Context, in DecideApprovalInput) (DecideApprovalOutput, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return DecideApprovalOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.RequestID) == "" {
		return DecideApprovalOutput{}, errors.New("request_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return DecideApprovalOutput{}, errors.New("trace_id is required")
	}
	decision := strings.TrimSpace(in.Decision)
	if decision == "" {
		return DecideApprovalOutput{}, errors.New("decision is required")
	}
	if decision != run.DecisionApprove && decision != run.DecisionReject {
		return DecideApprovalOutput{}, errors.New("decision must be approve or reject")
	}

	dbt := strings.TrimSpace(in.DecidedByType)
	if dbt == "" {
		dbt = run.ActorTypeUser
	}

	// 1) append decision ledger (append-only)
	d := run.ApprovalDecision{
		ProjectID:     strings.TrimSpace(in.ProjectID),
		RequestID:     strings.TrimSpace(in.RequestID),
		TraceID:       strings.TrimSpace(in.TraceID),
		Decision:      decision,
		DecidedByType: dbt,
		DecidedByID:   in.DecidedByID,
		Comment:       in.Comment,
		CreatedAt:     time.Now().UTC(),
	}
	if err := uc.ApprovalRepo.AppendDecision(ctx, d); err != nil {
		return DecideApprovalOutput{}, err
	}

	// 2) update request status
	if decision == run.DecisionApprove {
		if err := uc.ApprovalRepo.MarkApproved(ctx, in.RequestID); err != nil {
			return DecideApprovalOutput{}, err
		}
		return DecideApprovalOutput{
			ProjectID: d.ProjectID,
			RequestID: d.RequestID,
			TraceID:   d.TraceID,
			Decision:  decision,
			Status:    run.ApprovalStatusApproved,
		}, nil
	}

	if err := uc.ApprovalRepo.MarkRejected(ctx, in.RequestID); err != nil {
		return DecideApprovalOutput{}, err
	}
	return DecideApprovalOutput{
		ProjectID: d.ProjectID,
		RequestID: d.RequestID,
		TraceID:   d.TraceID,
		Decision:  decision,
		Status:    run.ApprovalStatusRejected,
	}, nil
}
