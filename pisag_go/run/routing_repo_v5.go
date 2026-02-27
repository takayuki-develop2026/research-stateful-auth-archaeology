package run

import "context"

// providers table
type ProviderRow struct {
	ProviderID  string
	ProjectID   string
	ProviderKey string
	Status      string // active|degraded|blocked
}

type ProvidersRepoV5 interface {
	ListActive(ctx context.Context, projectID string) ([]ProviderRow, error)
	GetByID(ctx context.Context, projectID, providerID string) (ProviderRow, error)
}

// provider_routes table
type ProviderRouteRow struct {
	RouteID       string
	ProjectID     string
	ProviderID    string
	Status        string // active|inactive|blocked
	Priority      int
	Region        string
	Currency      string
	PaymentMethod string
	Constraints   []byte // json bytes
	Weights       []byte // json bytes
	WhyPolicyRef  string
}

type ProviderRoutesRepoV5 interface {
	ListCandidates(ctx context.Context, projectID, region, currency, paymentMethod string) ([]ProviderRouteRow, error)
	GetByID(ctx context.Context, projectID, routeID string) (ProviderRouteRow, error)
}

// routing_metrics_daily table
type RoutingMetricsRepoV5 interface {
	GetLatestForRoute(ctx context.Context, projectID, routeID string) (*RoutingMetricSnapshot, error)
}

// route_decisions table
type RouteDecisionInsertInput struct {
	ProjectID string

	SubjectType       string
	SubjectInternalID string

	PolicyVersion   string
	PipelineVersion string
	RoutingVersion  string

	InputFingerprint string

	ChosenRouteID    *string
	ChosenProviderID *string

	FallbackUsed bool

	Status       string // chosen|review_required|denied
	DeniedReason *string

	WhyJSON        []byte // json bytes (lightweight)
	WhyEvidenceRef string // uuid

	UtlCommitEventKey string // varchar(128) with utl_internal: prefix

	TraceID string
	RunID   string // uuid
}

type RouteDecisionInsertResult struct {
	DecisionID     string // uuid
	FoundExisting  bool
}

type RouteDecisionsRepoV5 interface {
	InsertIfAbsent(ctx context.Context, in RouteDecisionInsertInput) (RouteDecisionInsertResult, error)
	GetByUnique(ctx context.Context, projectID, subjectType, subjectInternalID, policyVersion string) (RouteDecisionInsertInput, string /*decision_id*/, error)
}