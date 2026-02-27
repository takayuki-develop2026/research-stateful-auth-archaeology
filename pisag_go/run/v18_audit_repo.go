package run

import "context"

type AuditV18Repo interface {
	Append(ctx context.Context, in AuditAppendInput) (AuditAppendResult, error)
}

type AuditAppendInput struct {
	ProjectID string
	TraceID   string
	RunID     *string // varchar(26) in your DB

	Action string

	ActorType string // system|user|service
	ActorID   *string

	TargetType *string
	TargetID   *string

	Result string // ok|denied|failed
	Reason *string

	MetaJSON []byte // optional json bytes; if empty => NULL

	IdempotencyKey *string // optional but recommended
}

type AuditAppendResult struct {
	AuditEventID  int64
	FoundExisting bool
}
