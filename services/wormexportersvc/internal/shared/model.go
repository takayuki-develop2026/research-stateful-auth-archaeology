package shared

import "time"

type ComplianceEvent struct {
	ID                   int64
	ProjectID             string
	TraceID              string
	EventType            string
	EventEvidenceAssetID int64
	PrimaryArtifactID    *int64
	CreatedAtUTC         time.Time
}