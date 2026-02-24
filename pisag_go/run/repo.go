package run

import "context"

type RunRepo interface {
	Create(ctx context.Context, r Run) (Run, error)

	CreateOrGetByRunKey(
		ctx context.Context,
		projectID string,
		runKey string,
		newRun func() Run,
	) (r Run, foundExisting bool, err error)

	GetTraceID(ctx context.Context, runID string) (string, error)

	MarkDone(ctx context.Context, runID string) error
	MarkFailed(ctx context.Context, runID string, code string, msg string) error
}

type RunInputRepo interface {
	Insert(ctx context.Context, in RunInput) error
}

type RunEventRepo interface {
	Append(ctx context.Context, ev RunEvent) error
}

// v4.2+: claim repo returns trace_id with claimed input
type RunInputClaimRepo interface {
	ClaimNext(ctx context.Context, workerID string, style string) (*ClaimedRunInput, error)
	MarkDone(ctx context.Context, inputID int64, workerID string) error
	MarkRetry(ctx context.Context, inputID int64, workerID, code, msg string) error
	TouchClaim(ctx context.Context, inputID int64, workerID string) error
}