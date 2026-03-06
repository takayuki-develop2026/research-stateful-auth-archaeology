package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type AckMobileInboxItemUseCase struct {
	Devices  run.MobileDeviceRepository
	Inbox    run.MobileInboxRepository
	StepUps  run.MobileStepUpRepository
	Receipts run.MobileActionReceiptRepository
}

type AckMobileInboxItemInput struct {
	ProjectID         string
	ActorUserID       string
	DevicePublicID    string
	InboxItemPublicID string
	StepUpChallengeID string
	IdempotencyKey    string
	CommentText       string
	TraceID           string
}

type AckMobileInboxItemOutput struct {
	Item      run.MobileInboxItem
	Receipt   run.MobileActionReceipt
	Challenge *run.MobileStepUpChallenge
}

func (uc *AckMobileInboxItemUseCase) Handle(ctx context.Context, in AckMobileInboxItemInput) (AckMobileInboxItemOutput, error) {
	if uc.Devices == nil || uc.Inbox == nil || uc.Receipts == nil {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: repositories are not fully configured")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: actor_user_id is required")
	}
	if strings.TrimSpace(in.DevicePublicID) == "" {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: device_public_id is required")
	}
	if strings.TrimSpace(in.InboxItemPublicID) == "" {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: inbox_item_public_id is required")
	}

	if existing, ok := bestEffortV17FindReceiptByIdempotency(ctx, uc.Receipts, in.ProjectID, in.IdempotencyKey); ok {
		item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
		if err != nil {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item find existing item: %w", err)
		}
		return AckMobileInboxItemOutput{
			Item:    item,
			Receipt: existing,
		}, nil
	}

	traceID := ensureV17TraceID(in.TraceID)
	now := v17Now()

	device, err := loadV17ActiveOwnedDevice(ctx, uc.Devices, in.ProjectID, in.DevicePublicID, in.ActorUserID)
	if err != nil {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item load device: %w", err)
	}

	item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
	if err != nil {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item load inbox item: %w", err)
	}
	if !item.CanAck() && item.InboxStatus != run.MobileInboxStatusAcknowledged {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: ack_not_allowed")
	}
	if item.IsTerminal() {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: terminal_target")
	}

	var challenge *run.MobileStepUpChallenge
	var challengeID *int64

	if item.StepUpRequired {
		if uc.StepUps == nil {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: stepup repository is nil")
		}
		if strings.TrimSpace(in.StepUpChallengeID) == "" {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item: stepup challenge is required")
		}
		ch, err := loadAndValidateV17VerifiedChallengeForInboxAction(
			ctx,
			uc.StepUps,
			in.ProjectID,
			in.StepUpChallengeID,
			in.ActorUserID,
			device.ID,
			run.MobileActionAck,
			item,
			traceID,
			now,
		)
		if err != nil {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item validate stepup: %w", err)
		}
		consumed, err := uc.StepUps.MarkConsumed(ctx, run.ConsumeMobileStepUpChallengeInput{
			ProjectID:           in.ProjectID,
			ChallengePublicID:   ch.PublicID,
			ExpectedActorUserID: in.ActorUserID,
			ExpectedDeviceID:    device.ID,
			ExpectedActionKind:  run.MobileActionAck,
			ConsumedByUserID:    in.ActorUserID,
			ConsumedAt:          now,
			TraceID:             traceID,
			LastReasonCode:      "consumed_for_ack",
		})
		if err != nil {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item consume stepup: %w", err)
		}
		challenge = &consumed
		challengeID = &consumed.ID
	}

	receipt, err := uc.Receipts.Create(ctx, run.CreateMobileActionReceiptInput{
		PublicID:                newV17PublicID("mrcpt"),
		ProjectID:               in.ProjectID,
		ActionKind:              run.MobileActionAck,
		OutcomeStatus:           run.MobileActionOutcomeAttempted,
		ReasonCode:              "",
		MobileInboxItemID:       item.ID,
		MobileDeviceID:          device.ID,
		MobileStepUpChallengeID: challengeID,
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
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item create receipt: %w", err)
	}

	if item.InboxStatus == run.MobileInboxStatusAcknowledged {
		receipt, cerr := uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
			ProjectID:       in.ProjectID,
			ReceiptPublicID: receipt.PublicID,
			OutcomeStatus:   run.MobileActionOutcomeAlreadyApplied,
			ReasonCode:      "already_acknowledged",
			CompletedAt:     now,
			TraceID:         traceID,
		})
		if cerr != nil {
			return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item complete already applied receipt: %w", cerr)
		}
		return AckMobileInboxItemOutput{
			Item:      item,
			Receipt:   receipt,
			Challenge: challenge,
		}, nil
	}

	updatedItem, err := uc.Inbox.MarkAcknowledged(ctx, run.AcknowledgeMobileInboxItemInput{
		ProjectID:          in.ProjectID,
		InboxItemPublicID:  in.InboxItemPublicID,
		AcknowledgedAt:     now,
		TraceID:            traceID,
		TerminalReasonCode: "",
	})
	if err != nil {
		receipt, _ = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
			ProjectID:       in.ProjectID,
			ReceiptPublicID: receipt.PublicID,
			OutcomeStatus:   run.MobileActionOutcomeFailed,
			ReasonCode:      "ack_update_failed",
			CompletedAt:     now,
			TraceID:         traceID,
		})
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item mark acknowledged: %w", err)
	}

	receipt, err = uc.Receipts.Complete(ctx, run.CompleteMobileActionReceiptInput{
		ProjectID:       in.ProjectID,
		ReceiptPublicID: receipt.PublicID,
		OutcomeStatus:   run.MobileActionOutcomeSucceeded,
		ReasonCode:      "acknowledged",
		CompletedAt:     now,
		TraceID:         traceID,
	})
	if err != nil {
		return AckMobileInboxItemOutput{}, fmt.Errorf("ack mobile item complete receipt: %w", err)
	}

	return AckMobileInboxItemOutput{
		Item:      updatedItem,
		Receipt:   receipt,
		Challenge: challenge,
	}, nil
}
