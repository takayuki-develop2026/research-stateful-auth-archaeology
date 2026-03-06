package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	projectID := getenv("AK_PROJECT_ID", "default")
	actorUserID := getenv("AK_ACTOR_USER_ID", "admin")
	traceID := "trace_" + uuid.NewString()

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

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
	listInboxUC := &usecase.ListMobileInboxUseCase{
		Inbox: inbox,
	}
	approveUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}
	rejectUC := &usecase.RejectMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}
	ackUC := &usecase.AckMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	// ---------------------------------------------------------
	// 1. register device
	// ---------------------------------------------------------
	regOut, err := registerUC.Handle(ctx, usecase.RegisterMobileDeviceInput{
		ProjectID:           projectID,
		ActorUserID:         actorUserID,
		DeviceLabel:         "v17 smoke iphone",
		PlatformType:        run.MobilePlatformIOS,
		AppChannel:          run.MobileAppChannelPWA,
		DeviceFingerprint:   "fp_" + uuid.NewString(),
		DeviceKeyID:         "key_" + uuid.NewString(),
		DevicePublicKeyPEM:  "-----BEGIN PUBLIC KEY-----\nSMOKE\n-----END PUBLIC KEY-----",
		AttestationFormat:   "smoke",
		AttestationSubject:  "smoke-device",
		CreatedRunID:        "run_" + uuid.NewString(),
		TraceID:             traceID,
		ActivateImmediately: true,
	})
	if err != nil {
		log.Fatalf("register mobile device: %v", err)
	}
	fmt.Println("== register device OK ==")
	fmt.Printf("device_public_id=%s status=%s\n", regOut.Device.PublicID, regOut.Device.DeviceStatus)

	// ---------------------------------------------------------
	// 2. create inbox item directly (seed for smoke)
	// ---------------------------------------------------------
	inboxItem, err := inbox.Create(ctx, run.CreateMobileInboxItemInput{
		PublicID:               "minbox_" + uuid.NewString(),
		ProjectID:              projectID,
		InboxStatus:            run.MobileInboxStatusPending,
		ItemKind:               run.MobileInboxItemApprovalRequest,
		SourceType:             "approval_request",
		SourceID:               "apr_" + uuid.NewString(),
		RunID:                  "run_" + uuid.NewString(),
		TraceID:                traceID,
		ActorUserID:            actorUserID,
		AssignedUserID:         actorUserID,
		Priority:               run.MobilePriorityHigh,
		Severity:               run.MobileSeverityWarning,
		Title:                  "Smoke approval request",
		Summary:                "Approve this item in v17 smoke",
		ActionRequired:         true,
		StepUpRequired:         true,
		CommentRequired:        false,
		AvailableActionApprove: true,
		AvailableActionReject:  true,
		AvailableActionAck:     true,
		TerminalReasonCode:     "",
		SourceOccurredAt:       ptrTime(time.Now().UTC()),
		FirstPresentedAt:       ptrTime(time.Now().UTC()),
		DueAt:                  ptrTime(time.Now().UTC().Add(10 * time.Minute)),
	})
	if err != nil {
		log.Fatalf("create inbox item: %v", err)
	}
	fmt.Println("== create inbox item OK ==")
	fmt.Printf("inbox_public_id=%s source=%s/%s\n", inboxItem.PublicID, inboxItem.SourceType, inboxItem.SourceID)

	// ---------------------------------------------------------
	// 3. request approve stepup
	// ---------------------------------------------------------
	reqStepOut, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionApprove,
		InboxItemPublicID: inboxItem.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("request stepup: %v", err)
	}
	fmt.Println("== request stepup OK ==")
	fmt.Printf("challenge_public_id=%s otp=%s expires_at=%s\n",
		reqStepOut.Challenge.PublicID,
		reqStepOut.PlainChallengeValue,
		reqStepOut.ExpiresAt.Format(time.RFC3339),
	)

	// ---------------------------------------------------------
	// 4. verify approve stepup
	// ---------------------------------------------------------
	verifyOut, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqStepOut.Challenge.PublicID,
		ActionKind:        run.MobileActionApprove,
		VerificationValue: reqStepOut.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		log.Fatalf("verify stepup: %v", err)
	}
	fmt.Println("== verify stepup OK ==")
	fmt.Printf("challenge_status=%s\n", verifyOut.Challenge.ChallengeStatus)

	// ---------------------------------------------------------
	// 5. approve item
	// ---------------------------------------------------------
	approveOut, err := approveUC.Handle(ctx, usecase.ApproveMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: inboxItem.PublicID,
		StepUpChallengeID: verifyOut.Challenge.PublicID,
		IdempotencyKey:    "idem_" + uuid.NewString(),
		CommentText:       "approved by smoke",
		TraceID:           traceID,
	})
	if err != nil {
		log.Fatalf("approve mobile item: %v", err)
	}
	fmt.Println("== approve item OK ==")
	fmt.Printf("item_status=%s receipt_outcome=%s\n",
		approveOut.Item.InboxStatus,
		approveOut.Receipt.OutcomeStatus,
	)

	// ---------------------------------------------------------
	// 6. list inbox
	// ---------------------------------------------------------
	listOut, err := listInboxUC.Handle(ctx, usecase.ListMobileInboxInput{
		ProjectID:      projectID,
		AssignedUserID: actorUserID,
		OnlyActionable: false,
		Limit:          20,
		Offset:         0,
	})
	if err != nil {
		log.Fatalf("list inbox: %v", err)
	}
	fmt.Println("== list inbox OK ==")
	fmt.Printf("count=%d\n", len(listOut.Items))
	for i, item := range listOut.Items {
		fmt.Printf("[%d] public_id=%s status=%s title=%s\n", i, item.PublicID, item.InboxStatus, item.Title)
	}

	// ---------------------------------------------------------
	// 7. create ack-only inbox item and ack flow
	// ---------------------------------------------------------
	ackItem, err := inbox.Create(ctx, run.CreateMobileInboxItemInput{
		PublicID:               "minbox_" + uuid.NewString(),
		ProjectID:              projectID,
		InboxStatus:            run.MobileInboxStatusPending,
		ItemKind:               run.MobileInboxItemAlertAck,
		SourceType:             "alert",
		SourceID:               "alert_" + uuid.NewString(),
		RunID:                  "run_" + uuid.NewString(),
		TraceID:                traceID,
		ActorUserID:            actorUserID,
		AssignedUserID:         actorUserID,
		Priority:               run.MobilePriorityNormal,
		Severity:               run.MobileSeverityInfo,
		Title:                  "Smoke ack alert",
		Summary:                "Ack this item in v17 smoke",
		ActionRequired:         true,
		StepUpRequired:         false,
		CommentRequired:        false,
		AvailableActionApprove: false,
		AvailableActionReject:  false,
		AvailableActionAck:     true,
		TerminalReasonCode:     "",
		SourceOccurredAt:       ptrTime(time.Now().UTC()),
		FirstPresentedAt:       ptrTime(time.Now().UTC()),
		DueAt:                  ptrTime(time.Now().UTC().Add(5 * time.Minute)),
	})
	if err != nil {
		log.Fatalf("create ack item: %v", err)
	}

	ackOut, err := ackUC.Handle(ctx, usecase.AckMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: ackItem.PublicID,
		IdempotencyKey:    "idem_" + uuid.NewString(),
		CommentText:       "acked by smoke",
		TraceID:           traceID,
	})
	if err != nil {
		log.Fatalf("ack item: %v", err)
	}
	fmt.Println("== ack item OK ==")
	fmt.Printf("ack_item_status=%s ack_receipt=%s\n", ackOut.Item.InboxStatus, ackOut.Receipt.OutcomeStatus)

	// ---------------------------------------------------------
	// 8. create reject item and reject flow
	// ---------------------------------------------------------
	rejectItem, err := inbox.Create(ctx, run.CreateMobileInboxItemInput{
		PublicID:               "minbox_" + uuid.NewString(),
		ProjectID:              projectID,
		InboxStatus:            run.MobileInboxStatusPending,
		ItemKind:               run.MobileInboxItemManualDecision,
		SourceType:             "manual_decision",
		SourceID:               "md_" + uuid.NewString(),
		RunID:                  "run_" + uuid.NewString(),
		TraceID:                traceID,
		ActorUserID:            actorUserID,
		AssignedUserID:         actorUserID,
		Priority:               run.MobilePriorityHigh,
		Severity:               run.MobileSeverityWarning,
		Title:                  "Smoke reject decision",
		Summary:                "Reject this item in v17 smoke",
		ActionRequired:         true,
		StepUpRequired:         true,
		CommentRequired:        true,
		AvailableActionApprove: true,
		AvailableActionReject:  true,
		AvailableActionAck:     false,
		TerminalReasonCode:     "",
		SourceOccurredAt:       ptrTime(time.Now().UTC()),
		FirstPresentedAt:       ptrTime(time.Now().UTC()),
		DueAt:                  ptrTime(time.Now().UTC().Add(15 * time.Minute)),
	})
	if err != nil {
		log.Fatalf("create reject item: %v", err)
	}

	reqRejectStep, err := requestStepUpUC.Handle(ctx, usecase.RequestMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ActionKind:        run.MobileActionReject,
		InboxItemPublicID: rejectItem.PublicID,
		TraceID:           traceID,
		StepUpMethod:      run.MobileStepUpMethodOTP,
		TTL:               5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("request reject stepup: %v", err)
	}

	verifyRejectStep, err := verifyStepUpUC.Handle(ctx, usecase.VerifyMobileStepUpInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		ChallengePublicID: reqRejectStep.Challenge.PublicID,
		ActionKind:        run.MobileActionReject,
		VerificationValue: reqRejectStep.PlainChallengeValue,
		TraceID:           traceID,
	})
	if err != nil {
		log.Fatalf("verify reject stepup: %v", err)
	}

	rejectOut, err := rejectUC.Handle(ctx, usecase.RejectMobileInboxItemInput{
		ProjectID:         projectID,
		ActorUserID:       actorUserID,
		DevicePublicID:    regOut.Device.PublicID,
		InboxItemPublicID: rejectItem.PublicID,
		StepUpChallengeID: verifyRejectStep.Challenge.PublicID,
		IdempotencyKey:    "idem_" + uuid.NewString(),
		CommentText:       "rejected by smoke",
		TraceID:           traceID,
	})
	if err != nil {
		log.Fatalf("reject item: %v", err)
	}
	fmt.Println("== reject item OK ==")
	fmt.Printf("reject_item_status=%s reject_receipt=%s\n", rejectOut.Item.InboxStatus, rejectOut.Receipt.OutcomeStatus)

	fmt.Println("== v17 mobile smoke completed successfully ==")
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
