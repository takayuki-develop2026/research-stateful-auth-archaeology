package run

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Run struct {
	RunID           string
	ProjectID       string
	TraceID         string
	PipelineVersion string
	Status          Status

	// v4.2: run reuse key (project_id + run_key unique)
	RunKey *string

	StartedAt  time.Time
	FinishedAt *time.Time

	ErrorCode    *string
	ErrorMessage *string
}

// run_inputs table row (DB truth)
type RunInput struct {
	ID          int64
	RunID       string // uuid string
	SourceID    *string
	TargetURL   string
	Method      string
	HeadersJSON []byte // raw json bytes

	AllowlistKey *string

	// enqueue idempotency key (deterministic, NOT NULL in DB)
	EnqueueKey string
}

// work item returned by public.run_inputs_claim_next()
// (= run_inputs + runs.trace_id)
type ClaimedRunInput struct {
	RunInput
	TraceID string // uuid string (runs.trace_id)
}

type RunEvent struct {
	RunID     string // uuid string
	TraceID   string // uuid string
	EventName string
	Step      string
	Status    string
	Message   *string
	DataJSON  []byte // raw json bytes
}