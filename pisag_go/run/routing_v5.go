package run

import "time"

// -------- Request / Input --------

type RoutingInput struct {
	ProjectID string

	SubjectType       string // payment_intent|catalog_publish|...
	SubjectInternalID string // v7 internal id (required for v5)

	Region        string
	Currency      string
	PaymentMethod string
	AmountMinor   int64

	ConstraintsJSON []byte // optional (raw json bytes)
	PolicyVersion   string
	PipelineVersion string
	RoutingVersion  string // default "v5"

	TraceID string
	RunID   string // uuid string (commit should have run_id; preview may be empty but we recommend)
}

type RoutingPreviewInput struct {
	RoutingInput
}

type RoutingCommitInput struct {
	RoutingInput

	ExpectedInputFingerprint string

	AcceptSuggested bool
	OverrideRouteID *string // uuid string
}

// -------- Output --------

type RoutingCandidateScore struct {
	SuccessScore float64 `json:"success_score"`
	CostScore    float64 `json:"cost_score"`
	LatencyScore float64 `json:"latency_score"`
	TotalScore   float64 `json:"total_score"`
}

type RoutingCandidate struct {
	RouteID     string `json:"route_id"`
	ProviderID  string `json:"provider_id"`
	ProviderKey string `json:"provider_key"`

	Priority int `json:"priority"`

	Excluded       bool     `json:"excluded"`
	ExcludeReasons []string `json:"exclude_reasons"`

	Score RoutingCandidateScore `json:"score"`
}

type RoutingPreviewResult struct {
	Status string `json:"status"` // suggested|review_required|denied

	InputFingerprint string `json:"input_fingerprint"`

	Candidates []RoutingCandidate `json:"candidates"`

	SuggestedRouteID    *string `json:"suggested_route_id"`
	SuggestedProviderID *string `json:"suggested_provider_id"`

	DeniedReason *string `json:"denied_reason"`

	WhyEvidenceRef string `json:"why_evidence_ref"` // uuid

	TraceID string `json:"trace_id"`
	RunID   string `json:"run_id,omitempty"`
}

type RoutingCommitResult struct {
	DecisionID string `json:"decision_id"` // uuid

	Status string `json:"status"` // chosen|review_required|denied

	ChosenRouteID    *string `json:"chosen_route_id"`
	ChosenProviderID *string `json:"chosen_provider_id"`

	DeniedReason *string `json:"denied_reason"`

	WhyEvidenceRef    string `json:"why_evidence_ref"`
	UtlCommitEventKey string `json:"utl_commit_event_key"`

	InputFingerprint string `json:"input_fingerprint"`

	TraceID string `json:"trace_id"`
	RunID   string `json:"run_id"`
}

type RoutingMetricSnapshot struct {
	MetricDate   time.Time
	SuccessRate  float64
	P95LatencyMs int
	AvgCostMinor int64
	SampleN      int
}
