package run

import "context"

type MultimodalGeneratedOutput struct {
	EvidenceID int64
	OutputRole MultimodalOutputRole
	Seq        int
}

type OCRExecutionInput struct {
	Task MultimodalTask
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
}

type VisionExecutionInput struct {
	Task MultimodalTask
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
}

type OCRExecutionPort interface {
	ExecuteOCR(ctx context.Context, in OCRExecutionInput) (OCRExecutionOutput, error)
}

type VisionExecutionPort interface {
	ExecuteVision(ctx context.Context, in VisionExecutionInput) (VisionExecutionOutput, error)
}
