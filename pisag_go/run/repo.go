package run

import "context"

type RunRepo interface {
	Create(ctx context.Context, r Run) (Run, error)
	MarkDone(ctx context.Context, runID string) error
	MarkFailed(ctx context.Context, runID string, code string, msg string) error
}

type RunInputRepo interface {
	Insert(ctx context.Context, in RunInput) error
}

type RunEventRepo interface {
	Append(ctx context.Context, ev RunEvent) error
}
