package run

import "context"

type RunRepo interface {
	Create(ctx context.Context, r Run) (Run, error)

	// v4.2: run reuse (project_id + run_key unique)
	// foundExisting=true のとき既存runを返す
	CreateOrGetByRunKey(
		ctx context.Context,
		projectID string,
		runKey string,
		newRun func() Run,
	) (r Run, foundExisting bool, err error)

	// Optional helper (not required for worker loop if ClaimNext returns trace_id)
	GetTraceID(ctx context.Context, runID string) (string, error)

	// IMPORTANT: ak_worker cannot UPDATE runs directly.
	// Implementations must call SECURITY DEFINER functions:
	// - SELECT public.runs_mark_done($1)
	// - SELECT public.runs_mark_failed($1,$2,$3)
	MarkDone(ctx context.Context, runID string) error
	MarkFailed(ctx context.Context, runID string, code string, msg string) error
}

type RunInputRepo interface {
	Insert(ctx context.Context, in RunInput) error

	// v4.4: DB single-owner claim (public.run_inputs_claim_next)
	// returns nil, nil when no pending work
	ClaimNext(ctx context.Context, workerID string) (*ClaimedRunInput, error)
}

type RunEventRepo interface {
	Append(ctx context.Context, ev RunEvent) error
}