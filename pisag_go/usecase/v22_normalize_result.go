package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type NormalizeV22MultimodalResultUseCase struct {
	Results    run.MultimodalResultRepository
	Tasks      run.MultimodalTaskRepository
	Normalized run.NormalizedMultimodalResultRepository
}

type NormalizeV22MultimodalResultInput struct {
	ProjectID                        string
	ResultID                         int64
	NormalizedKind                   run.NormalizedMultimodalResultKind
	SummaryText                      string
	ConfidenceScore                  *float64
	ReasonCode                       string
	ReviewPayloadEvidenceAssetID     *int64
	DownstreamPayloadEvidenceAssetID *int64
}

type NormalizeV22MultimodalResultOutput struct {
	Result           run.MultimodalResult
	NormalizedResult run.NormalizedMultimodalResult
	Created          bool
}

func (uc *NormalizeV22MultimodalResultUseCase) Handle(ctx context.Context, in NormalizeV22MultimodalResultInput) (NormalizeV22MultimodalResultOutput, error) {
	if uc.Results == nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: results repository is nil")
	}
	if uc.Tasks == nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: tasks repository is nil")
	}
	if uc.Normalized == nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: normalized repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: project_id is required")
	}
	if in.ResultID <= 0 {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: result_id is required")
	}

	existing, err := uc.Normalized.FindByResultID(ctx, in.ProjectID, in.ResultID)
	if err == nil {
		result, err := uc.Results.FindByID(ctx, in.ResultID)
		if err != nil {
			return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result load result after existing: %w", err)
		}
		return NormalizeV22MultimodalResultOutput{
			Result:           result,
			NormalizedResult: existing,
			Created:          false,
		}, nil
	}

	result, err := uc.Results.FindByID(ctx, in.ResultID)
	if err != nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result load result: %w", err)
	}
	if result.ProjectID != in.ProjectID {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: result project mismatch")
	}

	task, err := uc.Tasks.FindByID(ctx, result.TaskID)
	if err != nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result load task: %w", err)
	}
	if task.ProjectID != in.ProjectID {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result: task project mismatch")
	}

	kind := in.NormalizedKind
	if kind == "" {
		kind = normalizedKindFromResultType(result.ResultType)
	}

	summary := strings.TrimSpace(in.SummaryText)
	if summary == "" {
		summary = defaultSummaryFromResultType(result.ResultType)
	}

	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "normalized"
	}

	reviewPayload := in.ReviewPayloadEvidenceAssetID
	if reviewPayload == nil {
		x := result.PayloadEvidenceAssetID
		reviewPayload = &x
	}

	downstreamPayload := in.DownstreamPayloadEvidenceAssetID
	if downstreamPayload == nil {
		x := result.PayloadEvidenceAssetID
		downstreamPayload = &x
	}

	norm, err := uc.Normalized.Create(ctx, run.CreateNormalizedMultimodalResultInput{
		ProjectID:                        in.ProjectID,
		TraceID:                          result.TraceID,
		RunID:                            result.RunID,
		TaskID:                           result.TaskID,
		ResultID:                         result.ID,
		NormalizedKind:                   kind,
		NormalizedStatus:                 run.NormalizedMultimodalResultStatusReady,
		SummaryText:                      summary,
		ConfidenceScore:                  in.ConfidenceScore,
		ReasonCode:                       reasonCode,
		ReviewPayloadEvidenceAssetID:     reviewPayload,
		DownstreamPayloadEvidenceAssetID: downstreamPayload,
	})
	if err != nil {
		return NormalizeV22MultimodalResultOutput{}, fmt.Errorf("normalize v22 multimodal result create: %w", err)
	}

	return NormalizeV22MultimodalResultOutput{
		Result:           result,
		NormalizedResult: norm,
		Created:          true,
	}, nil
}

func normalizedKindFromResultType(rt run.MultimodalResultType) run.NormalizedMultimodalResultKind {
	switch rt {
	case run.MultimodalResultTypePreprocessImage:
		return run.NormalizedMultimodalResultKindImagePreprocess

	case run.MultimodalResultTypeOCRText, run.MultimodalResultTypeExtractedText:
		return run.NormalizedMultimodalResultKindOCRText

	case run.MultimodalResultTypeDocParseStructure:
		return run.NormalizedMultimodalResultKindDocumentStructure

	case run.MultimodalResultTypeEmbeddingCandidates:
		return run.NormalizedMultimodalResultKindSimilarityCandidates

	case run.MultimodalResultTypeTranscript:
		return run.NormalizedMultimodalResultKindAudioTranscript

	case run.MultimodalResultTypeLLMText, run.MultimodalResultTypeLLMJSON:
		return run.NormalizedMultimodalResultKindLLMOutput

	case run.MultimodalResultTypeFusedMultimodal:
		return run.NormalizedMultimodalResultKindFusedMultimodal

	case run.MultimodalResultTypeVisionEntities:
		fallthrough
	default:
		return run.NormalizedMultimodalResultKindVisionEntity
	}
}

func defaultSummaryFromResultType(rt run.MultimodalResultType) string {
	switch rt {
	case run.MultimodalResultTypePreprocessImage:
		return "preprocess completed"
	case run.MultimodalResultTypeOCRText, run.MultimodalResultTypeExtractedText:
		return "ocr text normalized"
	case run.MultimodalResultTypeDocParseStructure:
		return "document structure normalized"
	case run.MultimodalResultTypeEmbeddingCandidates:
		return "embedding candidates normalized"
	case run.MultimodalResultTypeTranscript:
		return "audio transcript normalized"
	case run.MultimodalResultTypeLLMText:
		return "llm text normalized"
	case run.MultimodalResultTypeLLMJSON:
		return "llm json normalized"
	case run.MultimodalResultTypeFusedMultimodal:
		return "fused multimodal normalized"
	case run.MultimodalResultTypeVisionEntities:
		fallthrough
	default:
		return "vision entities normalized"
	}
}