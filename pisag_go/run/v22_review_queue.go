package run

import (
	"context"
	"time"
)

type MultimodalReviewQueueStatus string

const (
	MultimodalReviewQueueStatusPending    MultimodalReviewQueueStatus = "pending"
	MultimodalReviewQueueStatusProcessing MultimodalReviewQueueStatus = "processing"
	MultimodalReviewQueueStatusResolved   MultimodalReviewQueueStatus = "resolved"
)

type MultimodalReviewQueuePriority string

const (
	MultimodalReviewQueuePriorityLow    MultimodalReviewQueuePriority = "low"
	MultimodalReviewQueuePriorityNormal MultimodalReviewQueuePriority = "normal"
	MultimodalReviewQueuePriorityHigh   MultimodalReviewQueuePriority = "high"
)

type MultimodalReviewQueueItem struct {
	ID                 int64
	ProjectID          string
	TraceID            string
	RunID              string
	TaskID             int64
	ResultID           int64
	NormalizedResultID int64
	QueueStatus        MultimodalReviewQueueStatus
	Priority           MultimodalReviewQueuePriority
	ReasonCode         string
	AssignedReviewerID string
	CreatedAtUTC       time.Time
	UpdatedAtUTC       time.Time
	ResolvedAtUTC      *time.Time
}

type CreateMultimodalReviewQueueItemInput struct {
	ProjectID          string
	TraceID            string
	RunID              string
	TaskID             int64
	ResultID           int64
	NormalizedResultID int64
	QueueStatus        MultimodalReviewQueueStatus
	Priority           MultimodalReviewQueuePriority
	ReasonCode         string
	AssignedReviewerID string
}

type MultimodalReviewQueueRepository interface {
	Create(ctx context.Context, in CreateMultimodalReviewQueueItemInput) (MultimodalReviewQueueItem, error)
	FindByID(ctx context.Context, id int64) (MultimodalReviewQueueItem, error)
	ListByRunID(ctx context.Context, projectID, runID string) ([]MultimodalReviewQueueItem, error)
	ListPendingByProjectID(ctx context.Context, projectID string) ([]MultimodalReviewQueueItem, error)
	MarkResolved(ctx context.Context, projectID string, id int64, resolvedAtUTC time.Time) (MultimodalReviewQueueItem, error)
}
