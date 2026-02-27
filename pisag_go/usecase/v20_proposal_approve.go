package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

type V20ProposalApproveInput struct {
	ProjectID string
	TraceID   string

	ProposalID        int64
	ApprovalRequestID string
	ApprovedByUserID  *string
}

type V20ProposalApproveOutput struct {
	ProjectID  string
	TraceID    string
	ProposalID int64
	Status     string
}

type V20ProposalApproveUseCase struct {
	DB           *sql.DB
	ApprovalRepo run.ApprovalRepo
	V13Repo      *postgres.V13Repository // ★追加
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

	// ---- v13 idempotency start
	var idemID int64
	if uc.V13Repo != nil {
		scope := "v20_proposal_approve"
		key := "ak:idem:v20_proposal_approve:" + shortHash(fmt.Sprintf("%s|%d|%s", pid, in.ProposalID, reqID))
		fp := sha256Hex(fmt.Sprintf("%s|%d|%s", pid, in.ProposalID, reqID))
		start, err := uc.V13Repo.IdempotencyStart(ctx, pid, scope, key, fp)
		if err != nil {
			return V20ProposalApproveOutput{}, err
		}
		idemID = start.IdempotencyID

		// If already exists, check current proposal status and fast-return if already approved
		if start.FoundExisting {
			var st string
			err := uc.DB.QueryRowContext(ctx, `
SELECT status FROM public.remediation_proposals
WHERE id=$1 AND project_id=$2
LIMIT 1;
`, in.ProposalID, pid).Scan(&st)
			if err == nil && strings.ToLower(strings.TrimSpace(st)) == "approved" {
				sum := "already approved"
				_ = uc.V13Repo.IdempotencyFinish(ctx, pid, idemID, "succeeded", &sum, nil, time.Now().UTC())
				return V20ProposalApproveOutput{
					ProjectID: pid, TraceID: strings.TrimSpace(in.TraceID),
					ProposalID: in.ProposalID, Status: "approved",
				}, nil
			}
			// otherwise continue (may be needs_review)
		}
	}

	// 1) Guard (app-side): approval ledger must be approved
	req, err := uc.ApprovalRepo.GetRequest(ctx, pid, reqID)
	if err != nil {
		uc.idemFail(ctx, pid, idemID, err)
		return V20ProposalApproveOutput{}, err
	}
	if strings.ToLower(strings.TrimSpace(req.Status)) != strings.ToLower(run.ApprovalStatusApproved) {
		e := errors.New("approval_request is not approved")
		uc.idemFail(ctx, pid, idemID, e)
		return V20ProposalApproveOutput{}, e
	}

	approvedBy := "unknown"
	if in.ApprovedByUserID != nil && strings.TrimSpace(*in.ApprovedByUserID) != "" {
		approvedBy = strings.TrimSpace(*in.ApprovedByUserID)
	}

	// 2) DB-side gate
	const q = `SELECT public.proposal_mark_approved_v20($1,$2,$3::uuid,$4,$5);`
	if _, err := uc.DB.ExecContext(ctx, q, pid, in.ProposalID, reqID, approvedBy, time.Now().UTC()); err != nil {
		uc.idemFail(ctx, pid, idemID, err)
		return V20ProposalApproveOutput{}, err
	}

	if uc.V13Repo != nil && idemID > 0 {
		sum := "approved"
		_ = uc.V13Repo.IdempotencyFinish(ctx, pid, idemID, "succeeded", &sum, nil, time.Now().UTC())
	}

	return V20ProposalApproveOutput{
		ProjectID:  pid,
		TraceID:    strings.TrimSpace(in.TraceID),
		ProposalID: in.ProposalID,
		Status:     "approved",
	}, nil
}

func (uc *V20ProposalApproveUseCase) idemFail(ctx context.Context, projectID string, idemID int64, err error) {
	if uc.V13Repo == nil || idemID <= 0 {
		return
	}
	msg := err.Error()
	if len(msg) > 240 {
		msg = msg[:240]
	}
	_ = uc.V13Repo.IdempotencyFinish(ctx, projectID, idemID, "failed", &msg, nil, time.Now().UTC())
}
