package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type EnqueueV22MultimodalReviewUseCase struct {
	ReviewQueue run.MultimodalReviewQueueRepository
	Normalized  run.NormalizedMultimodalResultRepository
}

type EnqueueV22MultimodalReviewInput struct {
	ProjectID          string
	NormalizedResultID int64
	Priority           run.MultimodalReviewQueuePriority
	ReasonCode         string
	AssignedReviewerID string
}

type EnqueueV22MultimodalReviewOutput struct {
	NormalizedResult run.NormalizedMultimodalResult
	QueueItem        run.MultimodalReviewQueueItem
}

func (uc *EnqueueV22MultimodalReviewUseCase) Handle(ctx context.Context, in EnqueueV22MultimodalReviewInput) (EnqueueV22MultimodalReviewOutput, error) {
	if uc.ReviewQueue == nil {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review: review queue repository is nil")
	}
	if uc.Normalized == nil {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review: normalized repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review: project_id is required")
	}
	if in.NormalizedResultID <= 0 {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review: normalized_result_id is required")
	}

	projectID := strings.TrimSpace(in.ProjectID)

	norm, err := uc.Normalized.FindByID(ctx, in.NormalizedResultID)
	if err != nil {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review load normalized result: %w", err)
	}
	if norm.ProjectID != projectID {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review: normalized result project mismatch")
	}

	updatedNorm, err := uc.Normalized.UpdateStatus(ctx, projectID, norm.ID, run.NormalizedMultimodalResultStatusReviewRequired)
	if err != nil {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review update normalized status: %w", err)
	}

	priority := in.Priority
	if priority == "" {
		priority = run.MultimodalReviewQueuePriorityNormal
	}

	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "review_required"
	}

	item, err := uc.ReviewQueue.Create(ctx, run.CreateMultimodalReviewQueueItemInput{
		ProjectID:          updatedNorm.ProjectID,
		TraceID:            updatedNorm.TraceID,
		RunID:              updatedNorm.RunID,
		TaskID:             updatedNorm.TaskID,
		ResultID:           updatedNorm.ResultID,
		NormalizedResultID: updatedNorm.ID,
		QueueStatus:        run.MultimodalReviewQueueStatusPending,
		Priority:           priority,
		ReasonCode:         reasonCode,
		AssignedReviewerID: strings.TrimSpace(in.AssignedReviewerID),
	})
	if err != nil {
		return EnqueueV22MultimodalReviewOutput{}, fmt.Errorf("enqueue v22 multimodal review create queue item: %w", err)
	}

	return EnqueueV22MultimodalReviewOutput{
		NormalizedResult: updatedNorm,
		QueueItem:        item,
	}, nil
}