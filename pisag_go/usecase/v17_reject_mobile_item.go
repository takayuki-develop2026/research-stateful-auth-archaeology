package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RejectMobileInboxItemUseCase struct {
	Devices  run.MobileDeviceRepository
	Inbox    run.MobileInboxRepository
	StepUps  run.MobileStepUpRepository
	Receipts run.MobileActionReceiptRepository
}

type RejectMobileInboxItemInput struct {
	ProjectID         string
	ActorUserID       string
	DevicePublicID    string
	InboxItemPublicID string
	StepUpChallengeID string
	IdempotencyKey    string
	CommentText       string
	TraceID           string
}

type RejectMobileInboxItemOutput struct {
	Item      run.MobileInboxItem
	Receipt   run.MobileActionReceipt
	Challenge run.MobileStepUpChallenge
}

func (uc *RejectMobileInboxItemUseCase) Handle(ctx context.Context, in RejectMobileInboxItemInput) (RejectMobileInboxItemOutput, error) {
	if uc.Devices == nil || uc.Inbox == nil || uc.StepUps == nil || uc.Receipts == nil {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: repositories are not fully configured")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: actor_user_id is required")
	}
	if strings.TrimSpace(in.DevicePublicID) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: device_public_id is required")
	}
	if strings.TrimSpace(in.InboxItemPublicID) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: inbox_item_public_id is required")
	}
	if strings.TrimSpace(in.StepUpChallengeID) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: stepup challenge is required")
	}
	if strings.TrimSpace(in.CommentText) == "" {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: comment_text is required")
	}

	if existing, ok := bestEffortV17FindReceiptByIdempotency(ctx, uc.Receipts, in.ProjectID, in.IdempotencyKey); ok {
		item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
		if err != nil {
			return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item find existing item: %w", err)
		}
		challenge, _ := uc.StepUps.FindByPublicID(ctx, in.ProjectID, in.StepUpChallengeID)
		return RejectMobileInboxItemOutput{
			Item:      item,
			Receipt:   existing,
			Challenge: challenge,
		}, nil
	}

	traceID := ensureV17TraceID(in.TraceID)
	now := v17Now()

	device, err := loadV17ActiveOwnedDevice(ctx, uc.Devices, in.ProjectID, in.DevicePublicID, in.ActorUserID)
	if err != nil {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item load device: %w", err)
	}

	item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
	if err != nil {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item load inbox item: %w", err)
	}
	if item.IsTerminal() {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: terminal_target")
	}
	if !item.CanReject() {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item: reject_not_allowed")
	}

	challenge, err := loadAndValidateV17VerifiedChallengeForInboxAction(
		ctx,
		uc.StepUps,
		in.ProjectID,
		in.StepUpChallengeID,
		in.ActorUserID,
		device.ID,
		run.MobileActionReject,
		item,
		traceID,
		now,
	)
	if err != nil {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item validate stepup: %w", err)
	}

	receipt, err := uc.Receipts.Create(ctx, run.CreateMobileActionReceiptInput{
		PublicID:                newV17PublicID("mrcpt"),
		ProjectID:               in.ProjectID,
		ActionKind:              run.MobileActionReject,
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
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item create receipt: %w", err)
	}

	consumed, err := uc.StepUps.MarkConsumed(ctx, run.ConsumeMobileStepUpChallengeInput{
		ProjectID:           in.ProjectID,
		ChallengePublicID:   challenge.PublicID,
		ExpectedActorUserID: in.ActorUserID,
		ExpectedDeviceID:    device.ID,
		ExpectedActionKind:  run.MobileActionReject,
		ConsumedByUserID:    in.ActorUserID,
		ConsumedAt:          now,
		TraceID:             traceID,
		LastReasonCode:      "consumed_for_reject",
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
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item consume stepup: %w", err)
	}

	updatedItem, err := uc.Inbox.MarkRejected(ctx, run.RejectMobileInboxItemInput{
		ProjectID:          in.ProjectID,
		InboxItemPublicID:  in.InboxItemPublicID,
		RejectedAt:         now,
		TraceID:            traceID,
		TerminalReasonCode: "rejected_by_mobile",
	})
	if err != nil {
		receipt, _ = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
			ProjectID:       in.ProjectID,
			ReceiptPublicID: receipt.PublicID,
			OutcomeStatus:   run.MobileActionOutcomeFailed,
			ReasonCode:      "reject_update_failed",
			CompletedAt:     now,
			TraceID:         traceID,
		})
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item mark rejected: %w", err)
	}

	receipt, err = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
		ProjectID:       in.ProjectID,
		ReceiptPublicID: receipt.PublicID,
		OutcomeStatus:   run.MobileActionOutcomeSucceeded,
		ReasonCode:      "rejected",
		CompletedAt:     now,
		TraceID:         traceID,
	})
	if err != nil {
		return RejectMobileInboxItemOutput{}, fmt.Errorf("reject mobile item complete receipt: %w", err)
	}

	return RejectMobileInboxItemOutput{
		Item:      updatedItem,
		Receipt:   receipt,
		Challenge: consumed,
	}, nil
}
