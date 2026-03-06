package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func TestV17MobileApproveFlowIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV17TestPool(t)
	defer db.Close()

	projectID := "default"
	actorUserID := "admin"
	traceID := "trace_test_" + uuid.NewString()

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

	seedUC := &usecase.DevSeedMobileInboxUseCase{
		Inbox: inbox,
	}
	registerUC := &usecase.RegisterMobileDeviceUseCase{
		Devices: devices,
	}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}
	approveUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	seedOut, err := seedUC.Handle(ctx, usecase.DevSeedMobileInboxInput{
		ProjectID:       projectID,
		ActorUserID:     actorUserID,
		AssignedUserID:  actorUserID,
		ItemKind:        run.MobileInboxItemApprovalRequest,
		Priority:        run.MobilePriorityHigh,
		Severity:        run.MobileSeverityWarning,
		Title:           "integration approval item",
		Summary:         "seeded by integration test",
		StepUpRequired:  true,
		CommentRequired: false,
		CanApprove:      true,
		CanReject:       true,
		CanAck:          true,
		TraceID:         traceID,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}
	if seedOut.Item.InboxStatus != run.MobileInboxStatusPending {
		t.Fatalf("expected pending inbox item, got %s", seedOut.Item.InboxStatus)
	}

	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "integration iphone",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_it_" + uuid.NewString(),
		DeviceKeyID:         "key_it_" + uuid.NewString(),
		DevicePublicKeyPEM:  "-----BEGIN PUBLIC KEY-----\nTEST\n-----END PUBLIC KEY-----",
		AttestationFormat:   "integration",
		AttestationSubject:  "integration-device",
		CreatedRunID:        "run_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}
	if regOut.Device.DeviceStatus != run.MobileDeviceStatusActive {
		t.Fatalf("expected active device, got %s", regOut.Device.DeviceStatus)
	}
	if regOut.Device.ActivatedAt == nil {
		t.Fatalf("expected activated_at to be set")
	}

	reqOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: seedOut.Item.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("request stepup: %v", err)
	}
	if reqOut.Challenge.ChallengeStatus != run.MobileStepUpStatusIssued {
		t.Fatalf("expected issued challenge, got %s", reqOut.Challenge.ChallengeStatus)
	}
	if reqOut.PlainChallengeValue == "" {
		t.Fatalf("expected otp/plain challenge value")
	}

	verifyOut, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: reqOut.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("verify stepup: %v", err)
	}
	if verifyOut.Challenge.ChallengeStatus != run.MobileStepUpStatusVerified {
		t.Fatalf("expected verified challenge, got %s", verifyOut.Challenge.ChallengeStatus)
	}

	approveOut, err := approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    "idem_" + uuid.NewString(),
		CommentText:       "approved by integration",
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approveOut.Item.InboxStatus != run.MobileInboxStatusApproved {
		t.Fatalf("expected approved item, got %s", approveOut.Item.InboxStatus)
	}
	if approveOut.Receipt.OutcomeStatus != run.MobileActionOutcomeSucceeded {
		t.Fatalf("expected succeeded receipt, got %s", approveOut.Receipt.OutcomeStatus)
	}
	if approveOut.Challenge.ChallengeStatus != run.MobileStepUpStatusConsumed {
		t.Fatalf("expected consumed challenge, got %s", approveOut.Challenge.ChallengeStatus)
	}
}

func TestV17MobileWrongOTPIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV17TestPool(t)
	defer db.Close()

	projectID := "default"
	actorUserID := "admin"
	traceID := "trace_wrong_otp_" + uuid.NewString()

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)

	seedUC := &usecase.DevSeedMobileInboxUseCase{Inbox: inbox}
	registerUC := &usecase.RegisterMobileDeviceUseCase{Devices: devices}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}

	seedOut, err := seedUC.Handle(ctx, usecase.DevSeedMobileInboxInput{
		ProjectID:      projectID,
		ActorUserID:    actorUserID,
		AssignedUserID: actorUserID,
		ItemKind:       run.MobileInboxItemApprovalRequest,
		Title:          "wrong otp item",
		StepUpRequired: true,
		CanApprove:     true,
		CanReject:      true,
		CanAck:         true,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "wrong otp device",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_wrong_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	reqOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: seedOut.Item.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("request stepup: %v", err)
	}

	_, err = verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: "000000",
		TraceID:           traceID,
	})
	if err == nil {
		t.Fatalf("expected wrong otp verification to fail")
	}
}

func TestV17MobileAlreadyConsumedIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV17TestPool(t)
	defer db.Close()

	projectID := "default"
	actorUserID := "admin"
	traceID := "trace_consumed_" + uuid.NewString()

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

	seedUC := &usecase.DevSeedMobileInboxUseCase{Inbox: inbox}
	registerUC := &usecase.RegisterMobileDeviceUseCase{Devices: devices}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}
	approveUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	seedOut, err := seedUC.Handle(ctx, usecase.DevSeedMobileInboxInput{
		ProjectID:      projectID,
		ActorUserID:    actorUserID,
		AssignedUserID: actorUserID,
		ItemKind:       run.MobileInboxItemApprovalRequest,
		Title:          "already consumed item",
		StepUpRequired: true,
		CanApprove:     true,
		CanReject:      true,
		CanAck:         true,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "consumed device",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_consumed_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	reqOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: seedOut.Item.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("request stepup: %v", err)
	}

	verifyOut, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: reqOut.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("verify stepup: %v", err)
	}

	_, err = approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    "idem_first_" + uuid.NewString(),
		CommentText:       "first approve",
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}

	_, err = approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    "idem_second_" + uuid.NewString(),
		CommentText:       "second approve",
		TraceID:           traceID,
	})
	if err == nil {
		t.Fatalf("expected second approve with consumed challenge to fail")
	}
}

func TestV17MobileTerminalTargetIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV17TestPool(t)
	defer db.Close()

	projectID := "default"
	actorUserID := "admin"
	traceID := "trace_terminal_" + uuid.NewString()

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

	seedUC := &usecase.DevSeedMobileInboxUseCase{Inbox: inbox}
	registerUC := &usecase.RegisterMobileDeviceUseCase{Devices: devices}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}
	approveUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	seedOut, err := seedUC.Handle(ctx, usecase.DevSeedMobileInboxInput{
		ProjectID:      projectID,
		ActorUserID:    actorUserID,
		AssignedUserID: actorUserID,
		ItemKind:       run.MobileInboxItemApprovalRequest,
		Title:          "terminal item",
		StepUpRequired: true,
		CanApprove:     true,
		CanReject:      true,
		CanAck:         true,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "terminal device",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_terminal_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	reqOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: seedOut.Item.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("request stepup: %v", err)
	}

	verifyOut, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: reqOut.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("verify stepup: %v", err)
	}

	_, err = inbox.MarkCanceled(ctx, run.CancelMobileInboxItemInput{
		ProjectID:          projectID,
		InboxItemPublicID:  seedOut.Item.PublicID,
		CanceledAt:         time.Now().UTC(),
		TraceID:            traceID,
		TerminalReasonCode: "canceled_for_test",
	})
	if err != nil {
		t.Fatalf("cancel inbox item: %v", err)
	}

	_, err = approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    "idem_terminal_" + uuid.NewString(),
		CommentText:       "approve terminal target",
		TraceID:           traceID,
	})
	if err == nil {
		t.Fatalf("expected approve against terminal target to fail")
	}
}

func TestV17MobileApproveIdempotencyIntegration(t *testing.T) {
	ctx := context.Background()
	db := openV17TestPool(t)
	defer db.Close()

	projectID := "default"
	actorUserID := "admin"
	traceID := "trace_idem_" + uuid.NewString()

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

	seedUC := &usecase.DevSeedMobileInboxUseCase{Inbox: inbox}
	registerUC := &usecase.RegisterMobileDeviceUseCase{Devices: devices}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}
	approveUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	seedOut, err := seedUC.Handle(ctx, usecase.DevSeedMobileInboxInput{
		ProjectID:      projectID,
		ActorUserID:    actorUserID,
		AssignedUserID: actorUserID,
		ItemKind:       run.MobileInboxItemApprovalRequest,
		Title:          "idempotency item",
		StepUpRequired: true,
		CanApprove:     true,
		CanReject:      true,
		CanAck:         true,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "idempotency device",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_idem_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	reqOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: seedOut.Item.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("request stepup: %v", err)
	}

	verifyOut, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: reqOut.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("verify stepup: %v", err)
	}

	idemKey := "idem_same_" + uuid.NewString()

	first, err := approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    idemKey,
		CommentText:       "first idempotent approve",
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}

	second, err := approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: seedOut.Item.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    idemKey,
		CommentText:       "second idempotent approve",
		TraceID:           traceID,
	})
	if err != nil {
		t.Fatalf("second approve should return existing receipt, got error: %v", err)
	}

	if first.Receipt.PublicID != second.Receipt.PublicID {
		t.Fatalf("expected same receipt for same idempotency key, got %s vs %s", first.Receipt.PublicID, second.Receipt.PublicID)
	}
}

func openV17TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AK_PG_DSN")
	if dsn == "" {
		dsn = "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool new: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("pgxpool ping: %v", err)
	}
	return pool
}
