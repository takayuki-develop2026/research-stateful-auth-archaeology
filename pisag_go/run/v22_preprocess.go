package run

type PreprocessOperation string

const (
	PreprocessOperationOpenCVBasic PreprocessOperation = "opencv_basic"
	PreprocessOperationDeblur      PreprocessOperation = "deblur"
	PreprocessOperationDeskew      PreprocessOperation = "deskew"
	PreprocessOperationDenoise     PreprocessOperation = "denoise"
)

type PreprocessArtifactSummary struct {
	SourceEvidenceAssetID int64  `json:"source_evidence_asset_id"`
	OutputEvidenceAssetID int64  `json:"output_evidence_asset_id"`
	OutputKind            string `json:"output_kind"`
}

type PreprocessResult struct {
	TaskID                    int64                  `json:"task_id"`
	ProjectID                 string                 `json:"project_id"`
	EngineKind                EngineKind             `json:"engine_kind"`
	EngineVersion             string                 `json:"engine_version"`
	Operations                []PreprocessOperation  `json:"operations"`
	PayloadEvidenceAssetID    int64                  `json:"payload_evidence_asset_id"`
	ConfidenceEvidenceAssetID *int64                 `json:"confidence_evidence_asset_id,omitempty"`
	GeneratedOutputs          []MultimodalGeneratedOutput `json:"generated_outputs"`
	OutputHash                string                 `json:"output_hash"`
	SummaryText               string                 `json:"summary_text"`
	ConfidenceScore           *float64               `json:"confidence_score,omitempty"`
	ReasonCode                string                 `json:"reason_code"`
	ReviewRequired            bool                   `json:"review_required"`
	Metadata                  map[string]any         `json:"metadata,omitempty"`
}