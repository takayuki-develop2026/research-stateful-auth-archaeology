package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

// RequestApprovalInput creates an approval_request for a publish commit (v4.7).
// Idempotent by (project_id, commit_id).
type RequestApprovalInput struct {
	ProjectID string
	CommitID  string

	TraceID string

	RequestedByType string  // "system" | "user" (default system)
	RequestedByID   *string // nullable
	Reason          *string // nullable
}

type RequestApprovalOutput struct {
	RequestID     string
	ProjectID     string
	CommitID      string
	TraceID       string
	Status        string // pending/approved/rejected
	FoundExisting bool

	RequestedByType string
	RequestedByID   *string
	Reason          *string
}

type RequestApprovalUseCase struct {
	ApprovalRepo run.ApprovalRepo
}

func (uc *RequestApprovalUseCase) Handle(ctx context.Context, in RequestApprovalInput) (RequestApprovalOutput, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return RequestApprovalOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.CommitID) == "" {
		return RequestApprovalOutput{}, errors.New("commit_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return RequestApprovalOutput{}, errors.New("trace_id is required")
	}

	rbt := strings.TrimSpace(in.RequestedByType)
	if rbt == "" {
		rbt = run.ActorTypeSystem
	}

	// CreateOrGetPending is idempotent by (project_id, commit_id).
	req, found, err := uc.ApprovalRepo.CreateOrGetPending(
		ctx,
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.CommitID),
		strings.TrimSpace(in.TraceID),
		rbt,
		in.RequestedByID,
		in.Reason,
	)
	if err != nil {
		return RequestApprovalOutput{}, err
	}

	// v4.7 minimal: if already approved/rejected, keep as-is.
	_ = time.Now() // placeholder for future: emit run_events

	return RequestApprovalOutput{
		RequestID:       req.RequestID,
		ProjectID:       req.ProjectID,
		CommitID:        req.CommitID,
		TraceID:         req.TraceID,
		Status:          req.Status,
		FoundExisting:   found,
		RequestedByType: req.RequestedByType,
		RequestedByID:   req.RequestedByID,
		Reason:          req.Reason,
	}, nil
}