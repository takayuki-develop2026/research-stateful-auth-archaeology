package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type FuseV22MultimodalResultUseCase struct{}

type FuseV22MultimodalResultInput struct {
	ProjectID string
	TaskID    int64

	OCRResultID       *int64
	DocParseResultID  *int64
	EmbeddingResultID *int64
	VisionResultID    *int64
	LLMResultID       *int64

	OCRSummary       string
	DocParseSummary  string
	EmbeddingSummary string
	VisionSummary    string
	LLMSummary       string

	OCRConfidence       *float64
	DocParseConfidence  *float64
	EmbeddingConfidence *float64
	VisionConfidence    *float64
	LLMConfidence       *float64

	OCRReason       string
	DocParseReason  string
	EmbeddingReason string
	VisionReason    string
	LLMReason       string

	OCRReviewRequired       bool
	DocParseReviewRequired  bool
	EmbeddingReviewRequired bool
	VisionReviewRequired    bool
	LLMReviewRequired       bool
}

type FuseV22MultimodalResultOutput struct {
	Fused run.FusedMultimodalResult
}

func (uc *FuseV22MultimodalResultUseCase) Handle(ctx context.Context, in FuseV22MultimodalResultInput) (FuseV22MultimodalResultOutput, error) {
	_ = ctx

	if strings.TrimSpace(in.ProjectID) == "" {
		return FuseV22MultimodalResultOutput{}, fmt.Errorf("fuse v22 multimodal result: project_id is required")
	}
	if in.TaskID <= 0 {
		return FuseV22MultimodalResultOutput{}, fmt.Errorf("fuse v22 multimodal result: task_id is required")
	}

	summaries := make([]string, 0, 5)
	reasonParts := make([]string, 0, 5)
	disagreements := make([]run.FusedDisagreementFlag, 0, 4)
	reviewSignals := make([]run.ReviewSignal, 0, 4)

	if s := strings.TrimSpace(in.OCRSummary); s != "" {
		summaries = append(summaries, "ocr="+s)
	}
	if s := strings.TrimSpace(in.DocParseSummary); s != "" {
		summaries = append(summaries, "docparse="+s)
	}
	if s := strings.TrimSpace(in.EmbeddingSummary); s != "" {
		summaries = append(summaries, "embedding="+s)
	}
	if s := strings.TrimSpace(in.VisionSummary); s != "" {
		summaries = append(summaries, "vision="+s)
	}
	if s := strings.TrimSpace(in.LLMSummary); s != "" {
		summaries = append(summaries, "llm="+s)
	}

	if s := strings.TrimSpace(in.OCRReason); s != "" {
		reasonParts = append(reasonParts, "ocr:"+s)
	}
	if s := strings.TrimSpace(in.DocParseReason); s != "" {
		reasonParts = append(reasonParts, "docparse:"+s)
	}
	if s := strings.TrimSpace(in.EmbeddingReason); s != "" {
		reasonParts = append(reasonParts, "embedding:"+s)
	}
	if s := strings.TrimSpace(in.VisionReason); s != "" {
		reasonParts = append(reasonParts, "vision:"+s)
	}
	if s := strings.TrimSpace(in.LLMReason); s != "" {
		reasonParts = append(reasonParts, "llm:"+s)
	}

	if hasMeaningfulSummary(in.OCRSummary) && hasMeaningfulSummary(in.VisionSummary) && !sameSummary(in.OCRSummary, in.VisionSummary) {
		disagreements = appendUniqueDisagreement(disagreements, run.FusedDisagreementOCRVsVision)
	}
	if hasMeaningfulSummary(in.VisionSummary) && hasMeaningfulSummary(in.LLMSummary) && !sameSummary(in.VisionSummary, in.LLMSummary) {
		disagreements = appendUniqueDisagreement(disagreements, run.FusedDisagreementVisionVsLLM)
	}
	if hasMeaningfulSummary(in.OCRSummary) && hasMeaningfulSummary(in.DocParseSummary) && !sameSummary(in.OCRSummary, in.DocParseSummary) {
		disagreements = appendUniqueDisagreement(disagreements, run.FusedDisagreementOCRVsDocParse)
	}

	if len(disagreements) > 0 {
		reviewSignals = appendUniqueReviewSignal(reviewSignals, run.ReviewSignalDisagreement)
	}

	if lowConfidence(
		in.OCRConfidence,
		in.DocParseConfidence,
		in.EmbeddingConfidence,
		in.VisionConfidence,
		in.LLMConfidence,
	) {
		reviewSignals = appendUniqueReviewSignal(reviewSignals, run.ReviewSignalLowConfidence)
	}

	if in.OCRReviewRequired ||
		in.DocParseReviewRequired ||
		in.EmbeddingReviewRequired ||
		in.VisionReviewRequired ||
		in.LLMReviewRequired {
		reviewSignals = appendUniqueReviewSignal(reviewSignals, run.ReviewSignalManualReview)
	}

	fusedSummary := buildFusedSummary(summaries)
	confidence := maxConfidence(
		in.OCRConfidence,
		in.DocParseConfidence,
		in.EmbeddingConfidence,
		in.VisionConfidence,
		in.LLMConfidence,
	)
	reasonCode := buildReasonCode(reasonParts, reviewSignals)

	reviewRequired := len(reviewSignals) > 0

	fused := run.FusedMultimodalResult{
		TaskID:            in.TaskID,
		ProjectID:         in.ProjectID,
		SummaryText:       fusedSummary,
		ConfidenceScore:   confidence,
		ReasonCode:        reasonCode,
		ReviewRequired:    reviewRequired,
		DisagreementFlags: disagreements,
		ReviewSignals:     reviewSignals,
		OCRResultID:       in.OCRResultID,
		DocParseResultID:  in.DocParseResultID,
		EmbeddingResultID: in.EmbeddingResultID,
		VisionResultID:    in.VisionResultID,
		LLMResultID:       in.LLMResultID,
		Metadata: map[string]any{
			"summaries": map[string]any{
				"ocr":       nullIfBlank(in.OCRSummary),
				"docparse":  nullIfBlank(in.DocParseSummary),
				"embedding": nullIfBlank(in.EmbeddingSummary),
				"vision":    nullIfBlank(in.VisionSummary),
				"llm":       nullIfBlank(in.LLMSummary),
			},
			"reasons": map[string]any{
				"ocr":       nullIfBlank(in.OCRReason),
				"docparse":  nullIfBlank(in.DocParseReason),
				"embedding": nullIfBlank(in.EmbeddingReason),
				"vision":    nullIfBlank(in.VisionReason),
				"llm":       nullIfBlank(in.LLMReason),
			},
		},
	}

	return FuseV22MultimodalResultOutput{
		Fused: fused,
	}, nil
}

func buildFusedSummary(parts []string) string {
	if len(parts) == 0 {
		return "fused multimodal result"
	}
	return strings.Join(parts, " | ")
}

func buildReasonCode(reasonParts []string, reviewSignals []run.ReviewSignal) string {
	if len(reviewSignals) > 0 {
		switch {
		case containsReviewSignal(reviewSignals, run.ReviewSignalDisagreement):
			return "fused_disagreement"
		case containsReviewSignal(reviewSignals, run.ReviewSignalLowConfidence):
			return "fused_low_confidence"
		case containsReviewSignal(reviewSignals, run.ReviewSignalManualReview):
			return "fused_manual_review"
		default:
			return "fused_review_required"
		}
	}

	if len(reasonParts) == 0 {
		return "fused"
	}

	return "fused_ready"
}

func hasMeaningfulSummary(v string) bool {
	return strings.TrimSpace(v) != ""
}

func sameSummary(a, b string) bool {
	return normalizeSummary(a) == normalizeSummary(b)
}

func normalizeSummary(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func appendUniqueDisagreement(in []run.FusedDisagreementFlag, flag run.FusedDisagreementFlag) []run.FusedDisagreementFlag {
	for _, existing := range in {
		if existing == flag {
			return in
		}
	}
	return append(in, flag)
}

func appendUniqueReviewSignal(in []run.ReviewSignal, signal run.ReviewSignal) []run.ReviewSignal {
	for _, existing := range in {
		if existing == signal {
			return in
		}
	}
	return append(in, signal)
}

func containsReviewSignal(in []run.ReviewSignal, signal run.ReviewSignal) bool {
	for _, existing := range in {
		if existing == signal {
			return true
		}
	}
	return false
}

func lowConfidence(values ...*float64) bool {
	found := false
	for _, v := range values {
		if v == nil {
			continue
		}
		found = true
		if *v < 0.70 {
			return true
		}
	}
	return false && found
}

func maxConfidence(values ...*float64) *float64 {
	var max *float64
	for _, v := range values {
		if v == nil {
			continue
		}
		if max == nil || *v > *max {
			x := *v
			max = &x
		}
	}
	return max
}

func nullIfBlank(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}