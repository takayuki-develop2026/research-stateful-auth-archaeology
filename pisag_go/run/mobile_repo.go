package run

import "context"

type MobileDeviceRepository interface {
	Create(ctx context.Context, in RegisterMobileDeviceInput) (MobileDevice, error)
	FindByPublicID(ctx context.Context, projectID, publicID string) (MobileDevice, error)
	FindActiveByFingerprint(ctx context.Context, projectID, actorUserID, fingerprint string) (MobileDevice, error)
	ListByActor(ctx context.Context, projectID, actorUserID string) ([]MobileDevice, error)
	Activate(ctx context.Context, in ActivateMobileDeviceInput) (MobileDevice, error)
	UpdateLastSeen(ctx context.Context, in UpdateMobileDeviceLastSeenInput) error
	Disable(ctx context.Context, in DisableMobileDeviceInput) (MobileDevice, error)
	Revoke(ctx context.Context, in RevokeMobileDeviceInput) (MobileDevice, error)
	Rotate(ctx context.Context, in RotateMobileDeviceInput) (oldDevice MobileDevice, newDevice MobileDevice, err error)
}

type MobileStepUpRepository interface {
	Create(ctx context.Context, in IssueMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
	FindByPublicID(ctx context.Context, projectID, publicID string) (MobileStepUpChallenge, error)
	FindOpenByScope(ctx context.Context, projectID, actorUserID string, deviceID int64, scopeKind MobileStepUpScopeKind, actionKind MobileActionKind, targetInboxItemID *int64, targetSourceType, targetSourceID, runID string) (MobileStepUpChallenge, error)
	IncrementVerifyAttempt(ctx context.Context, in IncrementStepUpVerifyAttemptInput) (MobileStepUpChallenge, error)
	MarkVerified(ctx context.Context, in VerifyMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
	MarkConsumed(ctx context.Context, in ConsumeMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
	MarkExpired(ctx context.Context, in ExpireMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
	MarkRevoked(ctx context.Context, in RevokeMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
	MarkFailed(ctx context.Context, in FailMobileStepUpChallengeInput) (MobileStepUpChallenge, error)
}

type MobileInboxRepository interface {
	Create(ctx context.Context, in CreateMobileInboxItemInput) (MobileInboxItem, error)
	FindByPublicID(ctx context.Context, projectID, publicID string) (MobileInboxItem, error)
	FindOpenBySource(ctx context.Context, projectID, sourceType, sourceID string) (MobileInboxItem, error)
	List(ctx context.Context, filter ListMobileInboxItemsFilter) ([]MobileInboxItem, error)
	MarkAcknowledged(ctx context.Context, in AcknowledgeMobileInboxItemInput) (MobileInboxItem, error)
	MarkApproved(ctx context.Context, in ApproveMobileInboxItemInput) (MobileInboxItem, error)
	MarkRejected(ctx context.Context, in RejectMobileInboxItemInput) (MobileInboxItem, error)
	MarkExpired(ctx context.Context, in ExpireMobileInboxItemInput) (MobileInboxItem, error)
	MarkCanceled(ctx context.Context, in CancelMobileInboxItemInput) (MobileInboxItem, error)
	MarkSuperseded(ctx context.Context, in SupersedeMobileInboxItemInput) (MobileInboxItem, error)
}

type MobileActionReceiptRepository interface {
	Create(ctx context.Context, in CreateMobileActionReceiptInput) (MobileActionReceipt, error)
	FindByPublicID(ctx context.Context, projectID, publicID string) (MobileActionReceipt, error)
	FindByIdempotencyKey(ctx context.Context, in FindMobileActionReceiptByIdempotencyInput) (MobileActionReceipt, error)
	ListByInboxItem(ctx context.Context, projectID string, inboxItemID int64) ([]MobileActionReceipt, error)
	Complete(ctx context.Context, in CompleteMobileActionReceiptInput) (MobileActionReceipt, error)
}

type MobileRepositorySet struct {
	Devices  MobileDeviceRepository
	StepUps  MobileStepUpRepository
	Inbox    MobileInboxRepository
	Receipts MobileActionReceiptRepository
}
