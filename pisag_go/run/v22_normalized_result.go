package run

import (
	"context"
	"time"
)

type NormalizedMultimodalResultKind string

const (
	NormalizedMultimodalResultKindOCRText         NormalizedMultimodalResultKind = "ocr_text"
	NormalizedMultimodalResultKindVisionEntity    NormalizedMultimodalResultKind = "vision_entities"
	NormalizedMultimodalResultKindAudioTranscript NormalizedMultimodalResultKind = "audio_transcript"
)

type NormalizedMultimodalResultStatus string

const (
	NormalizedMultimodalResultStatusReady          NormalizedMultimodalResultStatus = "ready"
	NormalizedMultimodalResultStatusReviewRequired NormalizedMultimodalResultStatus = "review_required"
	NormalizedMultimodalResultStatusHandedOff      NormalizedMultimodalResultStatus = "handed_off"
)

type NormalizedMultimodalResult struct {
	ID        int64
	ProjectID string
	TraceID   string
	RunID     string
	TaskID    int64
	ResultID  int64

	NormalizedKind   NormalizedMultimodalResultKind
	NormalizedStatus NormalizedMultimodalResultStatus

	SummaryText     string
	ConfidenceScore *float64
	ReasonCode      string

	ReviewPayloadEvidenceAssetID     *int64
	DownstreamPayloadEvidenceAssetID *int64

	CreatedAtUTC time.Time
	UpdatedAtUTC time.Time
}

type CreateNormalizedMultimodalResultInput struct {
	ProjectID                        string
	TraceID                          string
	RunID                            string
	TaskID                           int64
	ResultID                         int64
	NormalizedKind                   NormalizedMultimodalResultKind
	NormalizedStatus                 NormalizedMultimodalResultStatus
	SummaryText                      string
	ConfidenceScore                  *float64
	ReasonCode                       string
	ReviewPayloadEvidenceAssetID     *int64
	DownstreamPayloadEvidenceAssetID *int64
}

type NormalizedMultimodalResultRepository interface {
	Create(ctx context.Context, in CreateNormalizedMultimodalResultInput) (NormalizedMultimodalResult, error)
	FindByID(ctx context.Context, id int64) (NormalizedMultimodalResult, error)
	FindByResultID(ctx context.Context, projectID string, resultID int64) (NormalizedMultimodalResult, error)
	ListByRunID(ctx context.Context, projectID, runID string) ([]NormalizedMultimodalResult, error)
	UpdateStatus(ctx context.Context, projectID string, id int64, status NormalizedMultimodalResultStatus) (NormalizedMultimodalResult, error)
}
