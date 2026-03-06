package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type ApproveMobileInboxItemUseCase struct {
	Devices  run.MobileDeviceRepository
	Inbox    run.MobileInboxRepository
	StepUps  run.MobileStepUpRepository
	Receipts run.MobileActionReceiptRepository
}

type ApproveMobileInboxItemInput struct {
	ProjectID         string
	ActorUserID       string
	DevicePublicID    string
	InboxItemPublicID string
	StepUpChallengeID string
	IdempotencyKey    string
	CommentText       string
	TraceID           string
}

type ApproveMobileInboxItemOutput struct {
	Item      run.MobileInboxItem
	Receipt   run.MobileActionReceipt
	Challenge run.MobileStepUpChallenge
}

func (uc *ApproveMobileInboxItemUseCase) Handle(ctx context.Context, in ApproveMobileInboxItemInput) (ApproveMobileInboxItemOutput, error) {
	if uc.Devices == nil || uc.Inbox == nil || uc.StepUps == nil || uc.Receipts == nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: repositories are not fully configured")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: actor_user_id is required")
	}
	if strings.TrimSpace(in.DevicePublicID) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: device_public_id is required")
	}
	if strings.TrimSpace(in.InboxItemPublicID) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: inbox_item_public_id is required")
	}
	if strings.TrimSpace(in.StepUpChallengeID) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: stepup challenge is required")
	}

	if existing, ok := bestEffortV17FindReceiptByIdempotency(ctx, uc.Receipts, in.ProjectID, in.IdempotencyKey); ok {
		item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
		if err != nil {
			return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item find existing item: %w", err)
		}
		challenge, _ := uc.StepUps.FindByPublicID(ctx, in.ProjectID, in.StepUpChallengeID)
		return ApproveMobileInboxItemOutput{
			Item:      item,
			Receipt:   existing,
			Challenge: challenge,
		}, nil
	}

	traceID := ensureV17TraceID(in.TraceID)
	now := v17Now()

	device, err := loadV17ActiveOwnedDevice(ctx, uc.Devices, in.ProjectID, in.DevicePublicID, in.ActorUserID)
	if err != nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item load device: %w", err)
	}

	item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
	if err != nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item load inbox item: %w", err)
	}
	if item.IsTerminal() {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: terminal_target")
	}
	if !item.CanApprove() {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: approve_not_allowed")
	}
	if item.CommentRequired && strings.TrimSpace(in.CommentText) == "" {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item: comment_required")
	}

	challenge, err := loadAndValidateV17VerifiedChallengeForInboxAction(
		ctx,
		uc.StepUps,
		in.ProjectID,
		in.StepUpChallengeID,
		in.ActorUserID,
		device.ID,
		run.MobileActionApprove,
		item,
		traceID,
		now,
	)
	if err != nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item validate stepup: %w", err)
	}

	receipt, err := uc.Receipts.Create(ctx, run.CreateMobileActionReceiptInput{
		PublicID:                newV17PublicID("mrcpt"),
		ProjectID:               in.ProjectID,
		ActionKind:              run.MobileActionApprove,
		OutcomeStatus:           run.MobileActionOutcomeAttempted,
		ReasonCode:              "",
		MobileInboxItemID:       item.ID,
		MobileDeviceID:          device.ID,
		MobileStepUpChallengeID: &challenge.ID,
		ActorUserID:             in.ActorUserID,
		SourceType:              item.SourceType,
		SourceID:                item.SourceID,
		RunID:                   item.RunID,
		TraceID:                 traceID,
		IdempotencyKey:          strings.TrimSpace(in.IdempotencyKey),
		CommentText:             strings.TrimSpace(in.CommentText),
		AttemptedAt:             now,
		CompletedAt:             nil,
	})
	if err != nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item create receipt: %w", err)
	}

	consumed, err := uc.StepUps.MarkConsumed(ctx, run.ConsumeMobileStepUpChallengeInput{
		ProjectID:           in.ProjectID,
		ChallengePublicID:   challenge.PublicID,
		ExpectedActorUserID: in.ActorUserID,
		ExpectedDeviceID:    device.ID,
		ExpectedActionKind:  run.MobileActionApprove,
		ConsumedByUserID:    in.ActorUserID,
		ConsumedAt:          now,
		TraceID:             traceID,
		LastReasonCode:      "consumed_for_approve",
	})
	if err != nil {
		receipt, _ = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
			ProjectID:       in.ProjectID,
			ReceiptPublicID: receipt.PublicID,
			OutcomeStatus:   run.MobileActionOutcomeDenied,
			ReasonCode:      "stepup_consume_failed",
			CompletedAt:     now,
			TraceID:         traceID,
		})
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item consume stepup: %w", err)
	}

	updatedItem, err := uc.Inbox.MarkApproved(ctx, run.ApproveMobileInboxItemInput{
		ProjectID:          in.ProjectID,
		InboxItemPublicID:  in.InboxItemPublicID,
		ApprovedAt:         now,
		TraceID:            traceID,
		TerminalReasonCode: "approved_by_mobile",
	})
	if err != nil {
		receipt, _ = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
			ProjectID:       in.ProjectID,
			ReceiptPublicID: receipt.PublicID,
			OutcomeStatus:   run.MobileActionOutcomeFailed,
			ReasonCode:      "approve_update_failed",
			CompletedAt:     now,
			TraceID:         traceID,
		})
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item mark approved: %w", err)
	}

	receipt, err = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
		ProjectID:       in.ProjectID,
		ReceiptPublicID: receipt.PublicID,
		OutcomeStatus:   run.MobileActionOutcomeSucceeded,
		ReasonCode:      "approved",
		CompletedAt:     now,
		TraceID:         traceID,
	})
	if err != nil {
		return ApproveMobileInboxItemOutput{}, fmt.Errorf("approve mobile item complete receipt: %w", err)
	}

	return ApproveMobileInboxItemOutput{
		Item:      updatedItem,
		Receipt:   receipt,
		Challenge: consumed,
	}, nil
}
