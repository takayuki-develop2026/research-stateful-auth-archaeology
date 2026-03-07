package run

import (
	"context"
	"time"
)

type MultimodalDownstreamHandoffStatus string

const (
	MultimodalDownstreamHandoffStatusPending   MultimodalDownstreamHandoffStatus = "pending"
	MultimodalDownstreamHandoffStatusDelivered MultimodalDownstreamHandoffStatus = "delivered"
	MultimodalDownstreamHandoffStatusFailed    MultimodalDownstreamHandoffStatus = "failed"
)

type MultimodalDownstreamHandoff struct {
	ID                     int64
	ProjectID              string
	TraceID                string
	RunID                  string
	TaskID                 int64
	ResultID               int64
	NormalizedResultID     int64
	DestinationKind        string
	PayloadEvidenceAssetID int64
	HandoffStatus          MultimodalDownstreamHandoffStatus
	ReasonCode             string
	CreatedAtUTC           time.Time
	UpdatedAtUTC           time.Time
	DeliveredAtUTC         *time.Time
}

type CreateMultimodalDownstreamHandoffInput struct {
	ProjectID              string
	TraceID                string
	RunID                  string
	TaskID                 int64
	ResultID               int64
	NormalizedResultID     int64
	DestinationKind        string
	PayloadEvidenceAssetID int64
	HandoffStatus          MultimodalDownstreamHandoffStatus
	ReasonCode             string
}

type MultimodalDownstreamHandoffRepository interface {
	Create(ctx context.Context, in CreateMultimodalDownstreamHandoffInput) (MultimodalDownstreamHandoff, error)
	FindByID(ctx context.Context, id int64) (MultimodalDownstreamHandoff, error)
	ListByRunID(ctx context.Context, projectID, runID string) ([]MultimodalDownstreamHandoff, error)
	MarkDelivered(ctx context.Context, projectID string, id int64, deliveredAtUTC time.Time) (MultimodalDownstreamHandoff, error)
}
