package run

import "time"

// ApprovalRequest represents an approval gate for a proposed publish commit (v4.7).
// default-deny: until approved, publish_confirm must NOT mark commit confirmed.
type ApprovalRequest struct {
	RequestID string // uuid string

	ProjectID string
	CommitID  string // uuid string

	TraceID string // uuid string

	// status: "pending" | "approved" | "rejected"
	Status string

	RequestedByType string  // "system" | "user"
	RequestedByID   *string // nullable
	Reason          *string // nullable

	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

const (
	ActorTypeSystem = "system"
	ActorTypeUser   = "user"
)

// ApprovalDecision is an append-only ledger of approve/reject actions.
type ApprovalDecision struct {
	DecisionID string // uuid string

	ProjectID string
	RequestID string // uuid string

	TraceID string // uuid string

	// decision: "approve" | "reject"
	Decision string

	DecidedByType string  // "system" | "user"
	DecidedByID   *string // nullable

	Comment *string // nullable

	CreatedAt time.Time
}

const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
)
