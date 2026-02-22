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

	StartedAt  time.Time
	FinishedAt *time.Time

	ErrorCode    *string
	ErrorMessage *string
}

type RunInput struct {
	RunID        string
	SourceID     *string
	TargetURL    string
	Method       string
	HeadersJSON  []byte // raw json bytes
	AllowlistKey *string
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