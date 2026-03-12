package run

type RuntimeRunDetail struct {
	Run               RuntimeRunSummary            `json:"run"`
	Task              RuntimeTaskSummary           `json:"task"`
	ModelRuns         []RuntimeModelRunSummary     `json:"model_runs"`
	Results           []RuntimeResultSummary       `json:"results"`
	NormalizedResult  *RuntimeNormalizedSummary    `json:"normalized_result,omitempty"`
	ReviewQueueItem   *RuntimeReviewQueueSummary   `json:"review_queue_item,omitempty"`
	DownstreamHandoffs []RuntimeDownstreamSummary  `json:"downstream_handoffs"`
	EvidenceRefs      []RuntimeEvidenceRef         `json:"evidence_refs"`
}

type RuntimeRunSummary struct {
	ID              int64          `json:"id"`
	ProjectID       string         `json:"project_id"`
	TraceID         string         `json:"trace_id"`
	RunID           string         `json:"run_id"`
	PipelineVersion string         `json:"pipeline_version"`
	Status          string         `json:"status"`
	StartedAt       *string        `json:"started_at,omitempty"`
	FinishedAt      *string        `json:"finished_at,omitempty"`
	ErrorCode       *string        `json:"error_code,omitempty"`
	ErrorMessage    *string        `json:"error_message,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type RuntimeTaskSummary struct {
	ID                        int64                  `json:"id"`
	ProjectID                 string                 `json:"project_id"`
	TraceID                   string                 `json:"trace_id"`
	RunID                     string                 `json:"run_id"`
	TaskKey                   string                 `json:"task_key"`
	TaskType                  string                 `json:"task_type"`
	PipelineVersion           string                 `json:"pipeline_version"`
	PolicyVersionStr          string                 `json:"policy_version_str"`
	InputHash                 string                 `json:"input_hash"`
	Status                    string                 `json:"status"`
	RouterPlanEvidenceAssetID int64                  `json:"router_plan_evidence_asset_id"`
	OptionsEvidenceAssetID    int64                  `json:"options_evidence_asset_id"`
	ModelRunID                *int64                 `json:"model_run_id,omitempty"`
	SoftErrorEvidenceAssetID  *int64                 `json:"soft_error_evidence_asset_id,omitempty"`
	StartedAtUTC              *string                `json:"started_at_utc,omitempty"`
	FinishedAtUTC             *string                `json:"finished_at_utc,omitempty"`
	CreatedAtUTC              string                 `json:"created_at_utc"`
	UpdatedAtUTC              string                 `json:"updated_at_utc"`
	EngineSelection           map[string][]string    `json:"engine_selection,omitempty"`
	Metadata                  map[string]any         `json:"metadata,omitempty"`
}

type RuntimeModelRunSummary struct {
	ID            int64          `json:"id"`
	TaskID        int64          `json:"task_id"`
	ProjectID     string         `json:"project_id"`
	Capability    string         `json:"capability"`
	EngineKind    string         `json:"engine_kind"`
	EngineVersion string         `json:"engine_version"`
	Provider      string         `json:"provider"`
	TaskKind      *string        `json:"task_kind,omitempty"`
	Status        string         `json:"status"`
	InputHash     string         `json:"input_hash"`
	StartedAt     string         `json:"started_at"`
	FinishedAt    *string        `json:"finished_at,omitempty"`
	LatencyMS     *int64         `json:"latency_ms,omitempty"`
	TokenUsage    map[string]any `json:"token_usage,omitempty"`
	CostEstimate  *float64       `json:"cost_estimate,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type RuntimeResultSummary struct {
	ID                        int64          `json:"id"`
	TaskID                    int64          `json:"task_id"`
	ProjectID                 string         `json:"project_id"`
	TraceID                   string         `json:"trace_id"`
	RunID                     string         `json:"run_id"`
	ResultKey                 string         `json:"result_key"`
	ResultType                string         `json:"result_type"`
	OutputHash                string         `json:"output_hash"`
	PayloadEvidenceAssetID    int64          `json:"payload_evidence_asset_id"`
	ConfidenceEvidenceAssetID *int64         `json:"confidence_evidence_asset_id,omitempty"`
	SummaryText               string         `json:"summary_text,omitempty"`
	ConfidenceScore           *float64       `json:"confidence_score,omitempty"`
	ReasonCode                string         `json:"reason_code,omitempty"`
	Metadata                  map[string]any `json:"metadata,omitempty"`
	CreatedAtUTC              string         `json:"created_at_utc"`
	UpdatedAtUTC              string         `json:"updated_at_utc"`
}

type RuntimeNormalizedSummary struct {
	ID                             int64          `json:"id"`
	ProjectID                      string         `json:"project_id"`
	TraceID                        string         `json:"trace_id"`
	RunID                          string         `json:"run_id"`
	TaskID                         int64          `json:"task_id"`
	ResultID                       int64          `json:"result_id"`
	NormalizedKind                 string         `json:"normalized_kind"`
	NormalizedStatus               string         `json:"normalized_status"`
	SummaryText                    string         `json:"summary_text"`
	ConfidenceScore                *float64       `json:"confidence_score,omitempty"`
	ReasonCode                     string         `json:"reason_code"`
	ReviewPayloadEvidenceAssetID   *int64         `json:"review_payload_evidence_asset_id,omitempty"`
	DownstreamPayloadEvidenceAssetID *int64       `json:"downstream_payload_evidence_asset_id,omitempty"`
	CreatedAtUTC                   string         `json:"created_at_utc"`
	UpdatedAtUTC                   string         `json:"updated_at_utc"`
	Metadata                       map[string]any `json:"metadata,omitempty"`
}

type RuntimeReviewQueueSummary struct {
	ID                 int64          `json:"id"`
	ProjectID          string         `json:"project_id"`
	NormalizedResultID int64          `json:"normalized_result_id"`
	Priority           string         `json:"priority"`
	Status             string         `json:"status"`
	ReasonCode         string         `json:"reason_code"`
	CreatedAtUTC       string         `json:"created_at_utc"`
	UpdatedAtUTC       string         `json:"updated_at_utc"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type RuntimeDownstreamSummary struct {
	ID                 int64          `json:"id"`
	ProjectID          string         `json:"project_id"`
	NormalizedResultID int64          `json:"normalized_result_id"`
	DestinationKind    string         `json:"destination_kind"`
	Status             string         `json:"status"`
	ReasonCode         string         `json:"reason_code"`
	CreatedAtUTC       string         `json:"created_at_utc"`
	UpdatedAtUTC       string         `json:"updated_at_utc"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type RuntimeEvidenceRef struct {
	ID        int64          `json:"id"`
	Kind      string         `json:"kind"`
	SHA256    string         `json:"sha256"`
	Bytes     int64          `json:"bytes"`
	CreatedAt string         `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}