package run

type LLMResult struct {
	TaskID                    int64                    `json:"task_id"`
	ProjectID                 string                   `json:"project_id"`
	EngineKind                EngineKind               `json:"engine_kind"`
	EngineVersion             string                   `json:"engine_version"`
	Provider                  EngineProvider           `json:"provider"`
	TaskKind                  LLMTaskKind              `json:"task_kind"`
	InputHash                 string                   `json:"input_hash"`
	OutputText                string                   `json:"output_text"`
	OutputJSON                map[string]any           `json:"output_json,omitempty"`
	RationaleText             string                   `json:"rationale_text,omitempty"`
	PromptVersion             string                   `json:"prompt_version"`
	TokenUsageJSON            map[string]any           `json:"token_usage_json,omitempty"`
	CostEstimate              *float64                 `json:"cost_estimate,omitempty"`
	PayloadEvidenceAssetID    int64                    `json:"payload_evidence_asset_id"`
	ConfidenceEvidenceAssetID *int64                   `json:"confidence_evidence_asset_id,omitempty"`
	GeneratedOutputs          []MultimodalGeneratedOutput `json:"generated_outputs"`
	OutputHash                string                   `json:"output_hash"`
	SummaryText               string                   `json:"summary_text"`
	ConfidenceScore           *float64                 `json:"confidence_score,omitempty"`
	ReasonCode                string                   `json:"reason_code"`
	ReviewRequired            bool                     `json:"review_required"`
	Metadata                  map[string]any           `json:"metadata,omitempty"`
}