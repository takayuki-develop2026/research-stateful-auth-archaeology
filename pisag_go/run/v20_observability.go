package run

import "time"

type SpanSummary struct {
	ID                   int64
	ProjectID            string
	TraceID              string // uuid as text
	SpanKey              string
	RunID                *string // uuid as text
	Service              string
	Operation            string
	Status               string // ok|error
	StartedAtUTC         time.Time
	EndedAtUTC           *time.Time
	SummaryEvidenceAssetID string // varchar(26)
}

type MetricRollup struct {
	ID                     int64
	ProjectID              string
	MetricKey              string
	TimeBucket             string // minute|hour|day
	BucketStartAtUTC       time.Time
	Value                  string // numeric -> string or float64 (choose later)
	DimensionsKey          string
	DimensionsEvidenceAssetID string
}

type SloDefinition struct {
	ID                        int64
	ProjectID                 string
	Name                      string
	Enabled                   bool
	WindowKind                string // 7d|30d
	Target                    string // numeric -> string (safe) or float64
	SloSpecEvidenceAssetID    string
	SeverityPolicyEvidenceAssetID string
	AlertPolicyEvidenceAssetID string
	CreatedByType             string
	CreatedByID               *string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type SloEvaluation struct {
	ID                    int64
	ProjectID             string
	SloID                 int64
	EvaluationKey         string
	WindowStartAtUTC      time.Time
	WindowEndAtUTC        time.Time
	SliValue              string
	ErrorBudgetRemaining  string
	Status                string // ok|warn|breach
	EvaluatedAtUTC        time.Time
	EvaluationEvidenceAssetID string
}

type Incident struct {
	ID                       int64
	ProjectID                string
	IncidentKey              string
	Status                   string
	Severity                 string
	IncidentType             string
	RootTraceID              *string // uuid as text
	RootRunID                *string // uuid as text
	DetectedBy               string
	DetectedAtUTC            time.Time
	IncidentSummaryEvidenceAssetID string
	PrimaryEvidenceAssetID   *string
	OwnerUserID              *string
	ResolvedAtUTC            *time.Time
}

type Proposal struct {
	ID                       int64
	ProjectID                string
	IncidentID               int64
	ProposalKey              string
	ProposalType             string
	Status                   string
	RiskLevel                string
	RequiresApproval         bool
	PlanEvidenceAssetID      string
	ImpactEvidenceAssetID    string
	PrimaryEvidenceAssetID   *string
	ApprovedByUserID         *string
	ApprovedAtUTC            *time.Time
	AppliedByUserID          *string
	AppliedAtUTC             *time.Time
	ExpiresAtUTC             *time.Time
}

type RemediationAction struct {
	ID                   int64
	ProjectID            string
	ProposalID           int64
	ActionKey            string
	RunID                string // uuid as text
	Status               string // queued|running|succeeded|failed
	ActionEvidenceAssetID string
	CreatedAtUTC         time.Time
	UpdatedAtUTC         time.Time
}