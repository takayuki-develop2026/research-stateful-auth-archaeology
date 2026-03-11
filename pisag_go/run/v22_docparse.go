package run

type DocParseBlock struct {
	Seq       int               `json:"seq"`
	BlockType string            `json:"block_type"`
	Text      string            `json:"text,omitempty"`
	BBox      []int             `json:"bbox,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

type DocParseTable struct {
	Seq      int               `json:"seq"`
	Rows     int               `json:"rows"`
	Cols     int               `json:"cols"`
	BBox     []int             `json:"bbox,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

type DocParseResult struct {
	TaskID                    int64                    `json:"task_id"`
	ProjectID                 string                   `json:"project_id"`
	EngineKind                EngineKind               `json:"engine_kind"`
	EngineVersion             string                   `json:"engine_version"`
	Blocks                    []DocParseBlock          `json:"blocks"`
	ReadingOrder              []int                    `json:"reading_order"`
	Tables                    []DocParseTable          `json:"tables"`
	MarkdownText              string                   `json:"markdown_text"`
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