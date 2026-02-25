package run

import "context"

// ApprovalRepo persists approval requests + decisions (v4.7).
type ApprovalRepo interface {
	// CreateOrGetPending creates an approval_request for (project_id, commit_id) idempotently.
	// If already exists, returns existing (foundExisting=true).
	CreateOrGetPending(
		ctx context.Context,
		projectID string,
		commitID string,
		traceID string,
		requestedByType string,
		requestedByID *string,
		reason *string,
	) (req ApprovalRequest, foundExisting bool, err error)

	// AppendDecision appends a decision row (approve/reject).
	AppendDecision(ctx context.Context, d ApprovalDecision) error

	// GetLatestStatus returns current status of the request ("pending"/"approved"/"rejected").
	GetLatestStatus(ctx context.Context, requestID string) (string, error)

	// MarkApproved/Rejected update request status (idempotent).
	MarkApproved(ctx context.Context, requestID string) error
	MarkRejected(ctx context.Context, requestID string) error

	// GetByProjectAndCommit fetches request by idempotency key.
	GetByProjectAndCommit(ctx context.Context, projectID string, commitID string) (ApprovalRequest, error)

	GetRequest(ctx context.Context, projectID, requestID string) (ApprovalRequest, error)
}