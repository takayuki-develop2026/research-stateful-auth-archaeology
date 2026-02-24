package run

import "context"

// PublishRepo persists publish commits (v4.6 SoT).
type PublishRepo interface {
	// CreateProposed inserts a proposed commit idempotently by (project_id, commit_key).
	// If already exists, it should return the existing row (foundExisting=true).
	CreateProposed(
		ctx context.Context,
		in PublishCommit,
	) (out PublishCommit, foundExisting bool, err error)

	// MarkConfirmed sets status=confirmed (idempotent).
	MarkConfirmed(ctx context.Context, commitID string) error

	// MarkFailed sets status=failed with error info (idempotent).
	MarkFailed(ctx context.Context, commitID string, code string, msg string) error

	// GetByProjectAndKey fetches a commit by idempotency key.
	GetByProjectAndKey(ctx context.Context, projectID string, commitKey string) (PublishCommit, error)
}