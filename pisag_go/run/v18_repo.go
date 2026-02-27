package run

import "context"

// v18 Evidence registry repo (calls evidence_register_v18)
type EvidenceV18Repo interface {
	Register(ctx context.Context, in EvidenceRegisterInput) (EvidenceRegisterResult, error)
}

type EvidenceRegisterInput struct {
	ProjectID string

	TraceID   string
	ActorType string // system|user|service
	ActorID   *string

	MediaType  string // text|image|audio|video|binary
	MimeType   string
	SourceKind string // pisag_fetch|upload|webhook|generated|import
	SourceURI  *string

	ContentSHA256 string // 64 hex
	ContentLength int64

	Language        *string
	RetentionPolicy string  // short|standard|legal_hold
	ExpiresAtUTC    *string // RFC3339Nano or nil

	IdempotencyKey string
}

type EvidenceRegisterResult struct {
	EvidenceRef   string // uuid string
	FoundExisting bool
}

// v18 Artifact registry repo (calls artifact_register_v18)
type ArtifactV18Repo interface {
	Register(ctx context.Context, in ArtifactRegisterInput) (ArtifactRegisterResult, error)
}

type ArtifactRegisterInput struct {
	ProjectID string

	ArtifactType  string // extracted_text|structured_json|embedding|thumbnail|transcript|features
	SchemaVersion string // e.g. extract.v1

	ContentSHA256 *string // optional
	ContentLength int64
	MimeType      string
	Status        string // active|orphaned|blocked (通常 active)

	IdempotencyKey string
}

type ArtifactRegisterResult struct {
	ArtifactRef   string // uuid string
	FoundExisting bool
}

// v18 Task type contract repo (calls task_type_contract_*_v18)
type TaskTypeContractV18Repo interface {
	Upsert(ctx context.Context, in TaskTypeContractUpsertInput) (TaskTypeContractChangeResult, error)
	Enable(ctx context.Context, in TaskTypeContractToggleInput) (TaskTypeContractChangeResult, error)
	Disable(ctx context.Context, in TaskTypeContractToggleInput) (TaskTypeContractChangeResult, error)
}

type TaskTypeContractUpsertInput struct {
	ProjectID       string
	TaskType        string
	PipelineVersion string

	PolicyVersionID *string
	Enabled         bool

	InputContractEvidenceRef  string
	OutputContractEvidenceRef string

	DefaultMode *string // Mode0..Mode4

	CreatedByType string // system|user|service
	CreatedByID   *string

	TraceID string
	RunID   *string // uuid string (optional)

	IdempotencyKey *string
}

type TaskTypeContractToggleInput struct {
	ProjectID       string
	TaskType        string
	PipelineVersion string

	TraceID       string
	CreatedByType string
	CreatedByID   *string
	RunID         *string

	IdempotencyKey *string
}

type TaskTypeContractChangeResult struct {
	ContractID    int64
	ChangeKind    string // created/updated/enabled/disabled
	FoundExisting bool
}

// v18 Links repo (calls *_link_add_v18)
type LinksV18Repo interface {
	AddRunEvidenceLink(ctx context.Context, projectID, runID, evidenceRef, role, idempotencyKey string) (LinkAddResult, error)
	AddRunArtifactLink(ctx context.Context, projectID, runID, artifactRef, role, idempotencyKey string) (LinkAddResult, error)
	AddArtifactEvidenceLink(ctx context.Context, projectID, artifactRef, evidenceRef, role, idempotencyKey string) (LinkAddResult, error)
}

type LinkAddResult struct {
	LinkID        int64
	FoundExisting bool
}
