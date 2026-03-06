package run

import "time"

type MobileActionOutcomeStatus string

const (
	MobileActionOutcomeAttempted      MobileActionOutcomeStatus = "attempted"
	MobileActionOutcomeSucceeded      MobileActionOutcomeStatus = "succeeded"
	MobileActionOutcomeDenied         MobileActionOutcomeStatus = "denied"
	MobileActionOutcomeExpired        MobileActionOutcomeStatus = "expired"
	MobileActionOutcomeFailed         MobileActionOutcomeStatus = "failed"
	MobileActionOutcomeAlreadyApplied MobileActionOutcomeStatus = "already_applied"
)

type MobileActionReceipt struct {
	ID                      int64
	PublicID                string
	ProjectID               string
	ActionKind              MobileActionKind
	OutcomeStatus           MobileActionOutcomeStatus
	ReasonCode              string
	MobileInboxItemID       int64
	MobileDeviceID          int64
	MobileStepUpChallengeID *int64
	ActorUserID             string
	SourceType              string
	SourceID                string
	RunID                   string
	TraceID                 string
	IdempotencyKey          string
	CommentText             string
	AttemptedAt             time.Time
	CompletedAt             *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type CreateMobileActionReceiptInput struct {
	PublicID                string
	ProjectID               string
	ActionKind              MobileActionKind
	OutcomeStatus           MobileActionOutcomeStatus
	ReasonCode              string
	MobileInboxItemID       int64
	MobileDeviceID          int64
	MobileStepUpChallengeID *int64
	ActorUserID             string
	SourceType              string
	SourceID                string
	RunID                   string
	TraceID                 string
	IdempotencyKey          string
	CommentText             string
	AttemptedAt             time.Time
	CompletedAt             *time.Time
}

type CompleteMobileActionReceiptInput struct {
	ProjectID       string
	ReceiptPublicID string
	OutcomeStatus   MobileActionOutcomeStatus
	ReasonCode      string
	CompletedAt     time.Time
	TraceID         string
}

type FindMobileActionReceiptByIdempotencyInput struct {
	ProjectID      string
	IdempotencyKey string
}

func (s MobileActionOutcomeStatus) IsTerminal() bool {
	return s == MobileActionOutcomeSucceeded ||
		s == MobileActionOutcomeDenied ||
		s == MobileActionOutcomeExpired ||
		s == MobileActionOutcomeFailed ||
		s == MobileActionOutcomeAlreadyApplied
}
