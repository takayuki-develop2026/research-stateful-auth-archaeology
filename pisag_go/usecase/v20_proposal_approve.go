package usecase

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type V20ProposalApproveInput struct {
	ProjectID string
	TraceID   string

	ProposalID int64

	// MUST be approved in v4.7 ledger and will be passed into DB gate
	ApprovalRequestID string

	ApprovedByUserID *string
}

type V20ProposalApproveOutput struct {
	ProjectID string
	TraceID   string

	ProposalID int64
	Status     string // approved
}

type V20ProposalApproveUseCase struct {
	DB           *sql.DB
	ApprovalRepo run.ApprovalRepo
}

func (uc *V20ProposalApproveUseCase) Handle(ctx context.Context, in V20ProposalApproveInput) (V20ProposalApproveOutput, error) {
	pid := strings.TrimSpace(in.ProjectID)
	if pid == "" {
		return V20ProposalApproveOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return V20ProposalApproveOutput{}, errors.New("trace_id is required")
	}
	if in.ProposalID <= 0 {
		return V20ProposalApproveOutput{}, errors.New("proposal_id is required")
	}

	reqID := strings.TrimSpace(in.ApprovalRequestID)
	if reqID == "" {
		return V20ProposalApproveOutput{}, errors.New("approval_request_id is required")
	}

	// 1) Guard (app-side): approval ledger must be approved
	req, err := uc.ApprovalRepo.GetRequest(ctx, pid, reqID)
	if err != nil {
		return V20ProposalApproveOutput{}, err
	}
	if strings.ToLower(strings.TrimSpace(req.Status)) != strings.ToLower(run.ApprovalStatusApproved) {
		return V20ProposalApproveOutput{}, errors.New("approval_request is not approved")
	}

	approvedBy := "unknown"
	if in.ApprovedByUserID != nil && strings.TrimSpace(*in.ApprovedByUserID) != "" {
		approvedBy = strings.TrimSpace(*in.ApprovedByUserID)
	}

	// 2) DB-side gate: MUST pass approval_request_id (uuid)
	const q = `SELECT public.proposal_mark_approved_v20($1,$2,$3::uuid,$4,$5);`
	if _, err := uc.DB.ExecContext(ctx, q, pid, in.ProposalID, reqID, approvedBy, time.Now().UTC()); err != nil {
		return V20ProposalApproveOutput{}, err
	}

	return V20ProposalApproveOutput{
		ProjectID:  pid,
		TraceID:    strings.TrimSpace(in.TraceID),
		ProposalID: in.ProposalID,
		Status:     "approved",
	}, nil
}