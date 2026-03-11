package run

type FusedDisagreementFlag string

const (
	FusedDisagreementOCRVsVision   FusedDisagreementFlag = "ocr_vs_vision"
	FusedDisagreementVisionVsLLM   FusedDisagreementFlag = "vision_vs_llm"
	FusedDisagreementOCRVsDocParse FusedDisagreementFlag = "ocr_vs_docparse"
)

type ReviewSignal string

const (
	ReviewSignalLowConfidence ReviewSignal = "low_confidence"
	ReviewSignalDisagreement  ReviewSignal = "disagreement"
	ReviewSignalPolicyHit     ReviewSignal = "policy_hit"
	ReviewSignalManualReview  ReviewSignal = "manual_review"
)

type FusedMultimodalResult struct {
	TaskID             int64                  `json:"task_id"`
	ProjectID          string                 `json:"project_id"`
	SummaryText        string                 `json:"summary_text"`
	ConfidenceScore    *float64               `json:"confidence_score,omitempty"`
	ReasonCode         string                 `json:"reason_code"`
	ReviewRequired     bool                   `json:"review_required"`
	DisagreementFlags  []FusedDisagreementFlag `json:"disagreement_flags,omitempty"`
	ReviewSignals      []ReviewSignal         `json:"review_signals,omitempty"`
	OCRResultID        *int64                 `json:"ocr_result_id,omitempty"`
	DocParseResultID   *int64                 `json:"docparse_result_id,omitempty"`
	EmbeddingResultID  *int64                 `json:"embedding_result_id,omitempty"`
	VisionResultID     *int64                 `json:"vision_result_id,omitempty"`
	LLMResultID        *int64                 `json:"llm_result_id,omitempty"`
	Metadata           map[string]any         `json:"metadata,omitempty"`
}