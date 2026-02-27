package run

import (
	"context"
	"time"
)

// -----------------------------
// Discovery: Sources
// -----------------------------

type SourceStatus string
type SourceFailureState string

const (
	SourceStatusDetected  SourceStatus = "detected"
	SourceStatusAcquired  SourceStatus = "acquired"
	SourceStatusExtracted SourceStatus = "extracted"

	SourceFailNone       SourceFailureState = "none"
	SourceFailNeedsRetry SourceFailureState = "needs_retry"
	SourceFailFailed     SourceFailureState = "failed"
)

type DiscoverySource struct {
	ID int64

	ProjectID string
	RunID     string // uuid string
	TraceID   string

	PipelineVersion string
	PolicyVersion   string

	SourceType   string
	SourceRefRaw string
	SourceRef    string
	SourceHash   string // 64hex

	Status       SourceStatus
	FailureState SourceFailureState
	FailureCode  *string
	FailureMsg   *string

	FirstSeenAt time.Time
	LastSeenAt  time.Time
	SeenCount   int64
}

type DiscoverySourceUpsertInput struct {
	ProjectID string
	RunID     string
	TraceID   string

	PipelineVersion string
	PolicyVersion   string

	SourceType   string
	SourceRefRaw string
	SourceRef    string
	SourceHash   string // 64hex

	// Optional: update progress/failure at same time
	Status       *SourceStatus
	FailureState *SourceFailureState
	FailureCode  *string
	FailureMsg   *string
}

type DiscoverySourceUpsertResult struct {
	SourceID      int64
	FoundExisting bool
}

type DiscoverySourceRepo interface {
	Upsert(ctx context.Context, in DiscoverySourceUpsertInput) (DiscoverySourceUpsertResult, error)
	Get(ctx context.Context, projectID string, sourceID int64) (DiscoverySource, error)
}

// -----------------------------
// Discovery: Candidates
// -----------------------------

type CandidateStatus string
type RiskLevel string

const (
	CandidateProposed       CandidateStatus = "proposed"
	CandidateReviewRequired CandidateStatus = "review_required"
	CandidateApproved       CandidateStatus = "approved"
	CandidateRejected       CandidateStatus = "rejected"
	CandidateApplied        CandidateStatus = "applied"
	CandidateNeedsRetry     CandidateStatus = "needs_retry"
	CandidateConflict       CandidateStatus = "conflict"

	RiskLow    RiskLevel = "low"
	RiskNormal RiskLevel = "normal"
	RiskHigh   RiskLevel = "high"
)

type DiscoveryCandidate struct {
	ID int64

	ProjectID string
	SourceID  int64

	CandidateType string
	CandidateKey  string // 64hex
	Status        CandidateStatus
	RiskLevel     RiskLevel
	Confidence    *float64

	PayloadEvidenceRef    *string // uuid
	NormalizedEvidenceRef *string // uuid
	DiffEvidenceRef       *string // uuid

	DedupeKey     *string // 64hex
	DedupeGroupID *int64

	FirstSeenAt time.Time
	LastSeenAt  time.Time
	SeenCount   int64

	ReviewRequestedAt *time.Time
	DecidedAt         *time.Time

	RunID           string
	TraceID         string
	PipelineVersion string
	PolicyVersion   string
}

type DiscoveryCandidateUpsertInput struct {
	ProjectID string
	SourceID  int64

	RunID           string
	TraceID         string
	PipelineVersion string
	PolicyVersion   string

	CandidateType string
	CandidateKey  string

	Status     *CandidateStatus
	RiskLevel  *RiskLevel
	Confidence *float64

	PayloadEvidenceRef    *string
	NormalizedEvidenceRef *string
	DiffEvidenceRef       *string
}

type DiscoveryCandidateUpsertResult struct {
	CandidateID   int64
	FoundExisting bool
}

type DiscoveryCandidateRepo interface {
	Upsert(ctx context.Context, in DiscoveryCandidateUpsertInput) (DiscoveryCandidateUpsertResult, error)
	Get(ctx context.Context, projectID string, candidateID int64) (DiscoveryCandidate, error)
	ListOps(ctx context.Context, projectID string, mode string, limit int) ([]DiscoveryCandidate, error) // stale|retry|apply_retry|archived
}

// -----------------------------
// Dedupe (v8.4)
// -----------------------------

type DedupeGroupStatus string

const (
	DedupeOpen           DedupeGroupStatus = "open"
	DedupeReviewRequired DedupeGroupStatus = "review_required"
	DedupeResolved       DedupeGroupStatus = "resolved"
)

type DedupeGroupUpsertInput struct {
	ProjectID string
	RunID     string
	TraceID   string

	CandidateType string
	DedupeKey     string // 64hex
}

type DedupeGroupUpsertResult struct {
	GroupID       int64
	FoundExisting bool
}

type DedupeGroupResolveInput struct {
	ProjectID string
	GroupID   int64
	RunID     string
	TraceID   string

	ResolutionType            string // choose_winner|merge_fields|reject_all
	WinnerCandidateID         *int64
	ResolutionNoteEvidenceRef *string
}

type DedupeRepo interface {
	UpsertGroup(ctx context.Context, in DedupeGroupUpsertInput) (DedupeGroupUpsertResult, error)
	LinkMember(ctx context.Context, projectID string, groupID, candidateID int64, memberRole, traceID, runID string) error
	MarkGroupReviewRequired(ctx context.Context, projectID string, groupID int64, traceID, runID string) error
	ResolveGroup(ctx context.Context, in DedupeGroupResolveInput) error
}

// -----------------------------
// Lifecycle (v8.7)
// -----------------------------

type LifecycleJobRunInput struct {
	ProjectID string
	RunID     string
	TraceID   string

	JobType string // mark_stale|schedule_retry|schedule_apply_retry|archive_expired|requeue_review
	Limit   int
	DryRun  bool
}

type LifecycleJobRunResult struct {
	JobID    int64
	Scanned  int64
	Changed  int64
	Archived int64
	Requeued int64
}

type LifecycleRepo interface {
	RunJob(ctx context.Context, in LifecycleJobRunInput) (LifecycleJobRunResult, error)
}
