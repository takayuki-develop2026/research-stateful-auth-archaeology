package run

import "time"

// EvidenceManifest represents the SoT declaration of "what evidence was collected for this run".
// One run -> at most one manifest (v4.5), status transitions: building -> complete.
type EvidenceManifest struct {
	ManifestID string // uuid string
	RunID      string // uuid string
	TraceID    string // uuid string

	Status string // "building" | "complete"

	// sha256 hex (64). nil while building, set when complete
	ManifestHash *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type EvidenceLink struct {
	ID int64

	ManifestID string // uuid string

	// kind: "fetch_body" / "fetch_headers" / "fetch_meta" ...
	Kind string

	// sha256 hex (64) referencing run_evidence_assets.sha256
	AssetSHA256 string

	// duplicated fields from assets for audit convenience
	ContentType *string
	ByteSize    int
	FinalURL    string
	StoredPath  string

	CreatedAt time.Time
}

// Common evidence kinds used by worker v4.5.
// Keep these as constants to avoid typos in code paths.
const (
	EvidenceKindFetchBody    = "fetch_body"
	EvidenceKindFetchHeaders = "fetch_headers"
	EvidenceKindFetchMeta    = "fetch_meta"
)
