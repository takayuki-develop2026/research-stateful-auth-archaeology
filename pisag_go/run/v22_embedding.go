package run

type SimilarityCandidate struct {
	Rank       int            `json:"rank"`
	CandidateID string        `json:"candidate_id"`
	Score      float64        `json:"score"`
	SourceKind string         `json:"source_kind,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type EmbeddingResult struct {
	TaskID                    int64                    `json:"task_id"`
	ProjectID                 string                   `json:"project_id"`
	EngineKind                EngineKind               `json:"engine_kind"`
	EngineVersion             string                   `json:"engine_version"`
	EmbeddingVectorRef        string                   `json:"embedding_vector_ref"`
	EmbeddingDim              int                      `json:"embedding_dim"`
	TopCandidates             []SimilarityCandidate    `json:"top_candidates"`
	PayloadEvidenceAssetID    int64                    `json:"payload_evidence_asset_id"`
	GeneratedOutputs          []MultimodalGeneratedOutput `json:"generated_outputs"`
	OutputHash                string                   `json:"output_hash"`
	SummaryText               string                   `json:"summary_text"`
	ConfidenceScore           *float64                 `json:"confidence_score,omitempty"`
	ReasonCode                string                   `json:"reason_code"`
	ReviewRequired            bool                     `json:"review_required"`
	Metadata                  map[string]any           `json:"metadata,omitempty"`
}