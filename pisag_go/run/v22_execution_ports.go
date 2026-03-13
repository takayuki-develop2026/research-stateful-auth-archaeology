package run

import "context"

type MultimodalGeneratedOutput struct {
	EvidenceID int64
	OutputRole MultimodalOutputRole
	Seq        int
}

type PreprocessExecutionInput struct {
	Task             MultimodalTask
	Selection        EngineSelection
	SourceEvidenceID *int64
}

type PreprocessExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
}

type OCRExecutionInput struct {
	Task             MultimodalTask
	Selection        EngineSelection
	SourceEvidenceID *int64
}

type OCRExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
}

type DocParseExecutionInput struct {
	Task      MultimodalTask
	Selection EngineSelection
}

type DocParseExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
}

type EmbeddingExecutionInput struct {
	Task      MultimodalTask
	Selection EngineSelection
}

type EmbeddingExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
}

type VisionExecutionInput struct {
	Task      MultimodalTask
	Selection EngineSelection
}

type VisionExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
}

type LLMExecutionInput struct {
	Task      MultimodalTask
	Selection EngineSelection
	TaskKind  LLMTaskKind
	Context   map[string]any
}

type LLMExecutionOutput struct {
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	GeneratedOutputs          []MultimodalGeneratedOutput
	OutputHash                string
	SummaryText               string
	ConfidenceScore           *float64
	ReasonCode                string
	ReviewRequired            bool
	EngineKind                EngineKind
	EngineVersion             string
	Metadata                  map[string]any
	OutputText                string
	OutputJSON                map[string]any
	RationaleText             string
	PromptVersion             string
	TokenUsageJSON            map[string]any
	CostEstimate              *float64
}

type PreprocessExecutionPort interface {
	ExecutePreprocess(ctx context.Context, in PreprocessExecutionInput) (PreprocessExecutionOutput, error)
}

type OCRExecutionPort interface {
	ExecuteOCR(ctx context.Context, in OCRExecutionInput) (OCRExecutionOutput, error)
}

type DocParseExecutionPort interface {
	ExecuteDocParse(ctx context.Context, in DocParseExecutionInput) (DocParseExecutionOutput, error)
}

type EmbeddingExecutionPort interface {
	ExecuteEmbedding(ctx context.Context, in EmbeddingExecutionInput) (EmbeddingExecutionOutput, error)
}

type VisionExecutionPort interface {
	ExecuteVision(ctx context.Context, in VisionExecutionInput) (VisionExecutionOutput, error)
}

type LLMExecutionPort interface {
	ExecuteLLM(ctx context.Context, in LLMExecutionInput) (LLMExecutionOutput, error)
}