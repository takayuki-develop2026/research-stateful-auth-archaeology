package run

type OCRTextBlock struct {
	Text   string   `json:"text"`
	Score  *float64 `json:"score,omitempty"`
	Box    []int    `json:"box,omitempty"`
	Seq    int      `json:"seq"`
	Role   string   `json:"role,omitempty"`
}

type OCRResult struct {
	TaskID                    int64                    `json:"task_id"`
	ProjectID                 string                   `json:"project_id"`
	EngineKind                EngineKind               `json:"engine_kind"`
	EngineVersion             string                   `json:"engine_version"`
	Language                  string                   `json:"language,omitempty"`
	RawText                   string                   `json:"raw_text"`
	Blocks                    []OCRTextBlock           `json:"blocks"`
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