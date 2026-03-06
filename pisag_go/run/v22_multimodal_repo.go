package run

import (
	"context"
	"time"
)

type MultimodalTaskRepository interface {
	Create(ctx context.Context, in RegisterMultimodalTaskInput) (MultimodalTask, error)
	FindByProjectAndTaskKey(ctx context.Context, projectID, taskKey string) (MultimodalTask, error)
	FindByID(ctx context.Context, id int64) (MultimodalTask, error)
	ListByRunID(ctx context.Context, projectID, runID string) ([]MultimodalTask, error)

	MarkRunning(ctx context.Context, projectID string, taskID int64, startedAtUTC time.Time) (MultimodalTask, error)
	MarkSucceeded(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time) (MultimodalTask, error)
	MarkReviewRequired(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (MultimodalTask, error)
	MarkSkippedBudget(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (MultimodalTask, error)
	MarkFailedSoft(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (MultimodalTask, error)
	MarkBlockedPolicy(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (MultimodalTask, error)
}

type MultimodalTaskInputRepository interface {
	Create(ctx context.Context, in AttachMultimodalTaskInputInput) (MultimodalTaskInput, error)
	ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]MultimodalTaskInput, error)
}

type MultimodalResultRepository interface {
	Create(ctx context.Context, in RegisterMultimodalResultInput) (MultimodalResult, error)
	FindByProjectAndResultKey(ctx context.Context, projectID, resultKey string) (MultimodalResult, error)
	FindByID(ctx context.Context, id int64) (MultimodalResult, error)
	ListByRunID(ctx context.Context, projectID, runID string) ([]MultimodalResult, error)
	ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]MultimodalResult, error)
}

type MultimodalResultOutputRepository interface {
	Create(ctx context.Context, in AttachMultimodalResultOutputInput) (MultimodalResultOutput, error)
	ListByResultID(ctx context.Context, projectID string, resultID int64) ([]MultimodalResultOutput, error)
}

type PIIRedactionRepository interface {
	Create(ctx context.Context, in RegisterPIIRedactionInput) (PIIRedaction, error)
	ListByEvidenceID(ctx context.Context, projectID string, evidenceID int64) ([]PIIRedaction, error)
	ListByTraceID(ctx context.Context, projectID, traceID string) ([]PIIRedaction, error)
}
