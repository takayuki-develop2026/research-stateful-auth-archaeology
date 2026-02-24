package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type ApprovalRepository struct{ db *sql.DB }

func NewApprovalRepository(db *sql.DB) *ApprovalRepository {
	return &ApprovalRepository{db: db}
}

func (r *ApprovalRepository) CreateOrGetPending(
	ctx context.Context,
	projectID string,
	commitID string,
	traceID string,
	requestedByType string,
	requestedByID *string,
	reason *string,
) (req run.ApprovalRequest, foundExisting bool, err error) {
	projectID = strings.TrimSpace(projectID)
	commitID = strings.TrimSpace(commitID)
	traceID = strings.TrimSpace(traceID)
	requestedByType = strings.TrimSpace(requestedByType)

	if projectID == "" {
		return run.ApprovalRequest{}, false, errors.New("project_id is required")
	}
	if commitID == "" {
		return run.ApprovalRequest{}, false, errors.New("commit_id is required")
	}
	if traceID == "" {
		return run.ApprovalRequest{}, false, errors.New("trace_id is required")
	}
	if requestedByType == "" {
		requestedByType = run.ActorTypeSystem
	}

	var requestedByIDAny any = nil
	if requestedByID != nil && strings.TrimSpace(*requestedByID) != "" {
		requestedByIDAny = strings.TrimSpace(*requestedByID)
	}

	var reasonAny any = nil
	if reason != nil && strings.TrimSpace(*reason) != "" {
		reasonAny = strings.TrimSpace(*reason)
	}

	// 1) insert idempotently
	const ins = `
INSERT INTO public.approval_requests
(project_id, commit_id, trace_id, status, requested_by_type, requested_by_id, reason)
VALUES ($1, $2::uuid, $3::uuid, 'pending', $4, $5, $6)
ON CONFLICT (project_id, commit_id) DO NOTHING
RETURNING
  request_id, project_id, commit_id, trace_id, status,
  requested_by_type, requested_by_id, reason,
  created_at, updated_at;
`
	var (
		requestID                              string
		pid                                    string
		cid                                    string
		tid                                    string
		status                                 string
		rbt                                    string
		rbid                                   sql.NullString
		rsn                                    sql.NullString
		createdAt, updatedAt                   time.Time
	)

	row := r.db.QueryRowContext(ctx, ins,
		projectID,
		commitID,
		traceID,
		requestedByType,
		requestedByIDAny,
		reasonAny,
	)

	scanErr := row.Scan(
		&requestID,
		&pid,
		&cid,
		&tid,
		&status,
		&rbt,
		&rbid,
		&rsn,
		&createdAt,
		&updatedAt,
	)

	if scanErr == nil {
		req = mapApprovalRequest(requestID, pid, cid, tid, status, rbt, rbid, rsn, createdAt, updatedAt)
		return req, false, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return run.ApprovalRequest{}, false, scanErr
	}

	// 2) conflict -> fetch existing
	ex, err := r.GetByProjectAndCommit(ctx, projectID, commitID)
	if err != nil {
		return run.ApprovalRequest{}, false, err
	}
	return ex, true, nil
}

func (r *ApprovalRepository) GetByProjectAndCommit(ctx context.Context, projectID string, commitID string) (run.ApprovalRequest, error) {
	projectID = strings.TrimSpace(projectID)
	commitID = strings.TrimSpace(commitID)

	if projectID == "" {
		return run.ApprovalRequest{}, errors.New("project_id is required")
	}
	if commitID == "" {
		return run.ApprovalRequest{}, errors.New("commit_id is required")
	}

	const q = `
SELECT
  request_id, project_id, commit_id, trace_id, status,
  requested_by_type, requested_by_id, reason,
  created_at, updated_at
FROM public.approval_requests
WHERE project_id=$1 AND commit_id=$2::uuid
LIMIT 1;
`
	var (
		requestID                              string
		pid                                    string
		cid                                    string
		tid                                    string
		status                                 string
		rbt                                    string
		rbid                                   sql.NullString
		rsn                                    sql.NullString
		createdAt, updatedAt                   time.Time
	)

	err := r.db.QueryRowContext(ctx, q, projectID, commitID).Scan(
		&requestID,
		&pid,
		&cid,
		&tid,
		&status,
		&rbt,
		&rbid,
		&rsn,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return run.ApprovalRequest{}, err
	}

	return mapApprovalRequest(requestID, pid, cid, tid, status, rbt, rbid, rsn, createdAt, updatedAt), nil
}

func (r *ApprovalRepository) AppendDecision(ctx context.Context, d run.ApprovalDecision) error {
	if strings.TrimSpace(d.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if strings.TrimSpace(d.RequestID) == "" {
		return errors.New("request_id is required")
	}
	if strings.TrimSpace(d.TraceID) == "" {
		return errors.New("trace_id is required")
	}
	if strings.TrimSpace(d.Decision) == "" {
		return errors.New("decision is required")
	}
	decidedByType := strings.TrimSpace(d.DecidedByType)
	if decidedByType == "" {
		decidedByType = run.ActorTypeUser
	}

	var decidedByIDAny any = nil
	if d.DecidedByID != nil && strings.TrimSpace(*d.DecidedByID) != "" {
		decidedByIDAny = strings.TrimSpace(*d.DecidedByID)
	}
	var commentAny any = nil
	if d.Comment != nil && strings.TrimSpace(*d.Comment) != "" {
		commentAny = strings.TrimSpace(*d.Comment)
	}

	const q = `
INSERT INTO public.approval_decisions
(project_id, request_id, trace_id, decision, decided_by_type, decided_by_id, comment)
VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6, $7);
`
	_, err := r.db.ExecContext(ctx, q,
		strings.TrimSpace(d.ProjectID),
		strings.TrimSpace(d.RequestID),
		strings.TrimSpace(d.TraceID),
		strings.TrimSpace(d.Decision),
		decidedByType,
		decidedByIDAny,
		commentAny,
	)
	return err
}

func (r *ApprovalRepository) GetLatestStatus(ctx context.Context, requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("request_id is required")
	}
	var status string
	err := r.db.QueryRowContext(ctx, `
SELECT status FROM public.approval_requests WHERE request_id=$1::uuid
`, requestID).Scan(&status)
	return status, err
}

func (r *ApprovalRepository) MarkApproved(ctx context.Context, requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return errors.New("request_id is required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE public.approval_requests
SET status='approved', updated_at=now()
WHERE request_id=$1::uuid
`, requestID)
	return err
}

func (r *ApprovalRepository) MarkRejected(ctx context.Context, requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return errors.New("request_id is required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE public.approval_requests
SET status='rejected', updated_at=now()
WHERE request_id=$1::uuid
`, requestID)
	return err
}

func mapApprovalRequest(
	requestID string,
	projectID string,
	commitID string,
	traceID string,
	status string,
	requestedByType string,
	requestedByID sql.NullString,
	reason sql.NullString,
	createdAt time.Time,
	updatedAt time.Time,
) run.ApprovalRequest {
	var rid *string
	if requestedByID.Valid && strings.TrimSpace(requestedByID.String) != "" {
		s := strings.TrimSpace(requestedByID.String)
		rid = &s
	}
	var rsn *string
	if reason.Valid && strings.TrimSpace(reason.String) != "" {
		s := strings.TrimSpace(reason.String)
		rsn = &s
	}

	return run.ApprovalRequest{
		RequestID:       requestID,
		ProjectID:       projectID,
		CommitID:        commitID,
		TraceID:         traceID,
		Status:          status,
		RequestedByType: requestedByType,
		RequestedByID:   rid,
		Reason:          rsn,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}