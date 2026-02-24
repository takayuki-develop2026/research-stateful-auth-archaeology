package run

import "time"

// PublishCommit is the SoT record for "publish confirmed" (v4.6).
// Idempotency is enforced by (project_id, commit_key) at DB layer.
//
// status:
// - proposed  : created; waiting for approval (v4.7) or later confirmation step
// - confirmed : publish confirmed (SoT)
// - failed    : publish failed (error_code/message set)
type PublishCommit struct {
	CommitID string // uuid string

	ProjectID string
	CommitKey string // sha256 hex(64) recommended (must NOT include run_id)

	ManifestID   string // uuid string
	ManifestHash string // sha256 hex(64)

	// traceability
	RunID   *string // uuid string (optional)
	TraceID string  // uuid string

	Target string // e.g. "catalog_v1"

	Status string // "proposed" | "confirmed" | "failed"

	ErrorCode    *string
	ErrorMessage *string

	MetaJSON []byte // raw json bytes

	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	PublishStatusProposed  = "proposed"
	PublishStatusConfirmed = "confirmed"
	PublishStatusFailed    = "failed"
)