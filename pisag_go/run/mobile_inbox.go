package run

import "time"

type MobileInboxStatus string

const (
	MobileInboxStatusPending      MobileInboxStatus = "pending"
	MobileInboxStatusAcknowledged MobileInboxStatus = "acknowledged"
	MobileInboxStatusApproved     MobileInboxStatus = "approved"
	MobileInboxStatusRejected     MobileInboxStatus = "rejected"
	MobileInboxStatusExpired      MobileInboxStatus = "expired"
	MobileInboxStatusCanceled     MobileInboxStatus = "canceled"
	MobileInboxStatusSuperseded   MobileInboxStatus = "superseded"
)

type MobileInboxItemKind string

const (
	MobileInboxItemApprovalRequest   MobileInboxItemKind = "approval_request"
	MobileInboxItemIncidentAck       MobileInboxItemKind = "incident_ack"
	MobileInboxItemAlertAck          MobileInboxItemKind = "alert_ack"
	MobileInboxItemReviewRequired    MobileInboxItemKind = "review_required"
	MobileInboxItemManualDecision    MobileInboxItemKind = "manual_decision"
	MobileInboxItemOperatorAttention MobileInboxItemKind = "operator_attention"
)

type MobilePriority string

const (
	MobilePriorityLow    MobilePriority = "low"
	MobilePriorityNormal MobilePriority = "normal"
	MobilePriorityHigh   MobilePriority = "high"
	MobilePriorityUrgent MobilePriority = "urgent"
)

type MobileSeverity string

const (
	MobileSeverityInfo     MobileSeverity = "info"
	MobileSeverityWarning  MobileSeverity = "warning"
	MobileSeverityCritical MobileSeverity = "critical"
)

type MobileInboxItem struct {
	ID                     int64
	PublicID               string
	ProjectID              string
	InboxStatus            MobileInboxStatus
	ItemKind               MobileInboxItemKind
	SourceType             string
	SourceID               string
	RunID                  string
	TraceID                string
	ActorUserID            string
	AssignedUserID         string
	Priority               MobilePriority
	Severity               MobileSeverity
	Title                  string
	Summary                string
	ActionRequired         bool
	StepUpRequired         bool
	CommentRequired        bool
	AvailableActionApprove bool
	AvailableActionReject  bool
	AvailableActionAck     bool
	TerminalAt             *time.Time
	TerminalReasonCode     string
	SourceOccurredAt       *time.Time
	FirstPresentedAt       *time.Time
	FirstAcknowledgedAt    *time.Time
	ApprovedAt             *time.Time
	RejectedAt             *time.Time
	ExpiredAt              *time.Time
	CanceledAt             *time.Time
	SupersededAt           *time.Time
	DueAt                  *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (i MobileInboxItem) IsTerminal() bool {
	switch i.InboxStatus {
	case MobileInboxStatusApproved,
		MobileInboxStatusRejected,
		MobileInboxStatusExpired,
		MobileInboxStatusCanceled,
		MobileInboxStatusSuperseded:
		return true
	default:
		return false
	}
}

func (i MobileInboxItem) CanApprove() bool {
	return !i.IsTerminal() && i.AvailableActionApprove
}

func (i MobileInboxItem) CanReject() bool {
	return !i.IsTerminal() && i.AvailableActionReject
}

func (i MobileInboxItem) CanAck() bool {
	return !i.IsTerminal() && i.AvailableActionAck
}

type CreateMobileInboxItemInput struct {
	PublicID               string
	ProjectID              string
	InboxStatus            MobileInboxStatus
	ItemKind               MobileInboxItemKind
	SourceType             string
	SourceID               string
	RunID                  string
	TraceID                string
	ActorUserID            string
	AssignedUserID         string
	Priority               MobilePriority
	Severity               MobileSeverity
	Title                  string
	Summary                string
	ActionRequired         bool
	StepUpRequired         bool
	CommentRequired        bool
	AvailableActionApprove bool
	AvailableActionReject  bool
	AvailableActionAck     bool
	TerminalReasonCode     string
	SourceOccurredAt       *time.Time
	FirstPresentedAt       *time.Time
	DueAt                  *time.Time
}

type ListMobileInboxItemsFilter struct {
	ProjectID      string
	AssignedUserID string
	ActorUserID    string
	Status         MobileInboxStatus
	ItemKind       MobileInboxItemKind
	Priority       MobilePriority
	Severity       MobileSeverity
	OnlyActionable bool
	Limit          int
	Offset         int
}

type AcknowledgeMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	AcknowledgedAt     time.Time
	TraceID            string
	TerminalReasonCode string
}

type ApproveMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	ApprovedAt         time.Time
	TraceID            string
	TerminalReasonCode string
}

type RejectMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	RejectedAt         time.Time
	TraceID            string
	TerminalReasonCode string
}

type ExpireMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	ExpiredAt          time.Time
	TraceID            string
	TerminalReasonCode string
}

type CancelMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	CanceledAt         time.Time
	TraceID            string
	TerminalReasonCode string
}

type SupersedeMobileInboxItemInput struct {
	ProjectID          string
	InboxItemPublicID  string
	SupersededAt       time.Time
	TraceID            string
	TerminalReasonCode string
}
