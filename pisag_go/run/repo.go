package run

import "context"

type RunRepo interface {
	Create(ctx context.Context, r Run) (Run, error)

	// v4.2: run_id固定再実行（同一目的runを再利用）
	// foundExisting=true のとき既存runを返す
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