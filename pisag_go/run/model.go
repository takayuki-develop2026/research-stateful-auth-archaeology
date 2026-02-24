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

	RunKey *string

	StartedAt  time.Time
	FinishedAt *time.Time

	ErrorCode    *string
	ErrorMessage *string
}

type RunInput struct {
	ID          int64
	RunID       string
	TraceID     string
	SourceID    *string
	TargetURL   string
	Method      string
	HeadersJSON []byte // raw json bytes

	AllowlistKey *string

	// enqueue idempotency key (deterministic)
	EnqueueKey string
}

type RunEvent struct {
	RunID     string
	TraceID   string
	EventName string
	Step      string
	Status    string
	Message   *string
	DataJSON  []byte // raw json bytes
}