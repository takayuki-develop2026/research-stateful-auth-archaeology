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

	// MUST be approved in v4.7 ledger (approval_requests.status=approved)
	ApprovalRequestID string

	// Who is finalizing proposal approval (for v20 table fields)
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
	rid := strings.TrimSpace(in.ApprovalRequestID)
	if rid == "" {
		return V20ProposalApproveOutput{}, errors.New("approval_request_id is required")
	}

	// 1) Guard: approval ledger must be approved (default deny)
	req, err := uc.ApprovalRepo.GetRequest(ctx, pid, rid)
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

	// 2) Mark proposal approved (EXECUTE ONLY)
	const q = `SELECT public.proposal_mark_approved_v20($1,$2,$3,$4);`
	if _, err := uc.DB.ExecContext(ctx, q, pid, in.ProposalID, approvedBy, time.Now().UTC()); err != nil {
		return V20ProposalApproveOutput{}, err
	}

	return V20ProposalApproveOutput{
		ProjectID:  pid,
		TraceID:    strings.TrimSpace(in.TraceID),
		ProposalID: in.ProposalID,
		Status:     "approved",
	}, nil
}