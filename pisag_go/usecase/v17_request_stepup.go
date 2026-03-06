package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	run "example.com/pisag_go/run"

	"github.com/google/uuid"
)

type RequestMobileStepUpUseCase struct {
	Devices run.MobileDeviceRepository
	Inbox   run.MobileInboxRepository
	StepUps run.MobileStepUpRepository
}

type RequestMobileStepUpInput struct {
	ProjectID         string
	ActorUserID       string
	DevicePublicID    string
	ActionKind        run.MobileActionKind
	InboxItemPublicID string
	TargetSourceType  string
	TargetSourceID    string
	RunID             string
	TraceID           string

	StepUpMethod run.MobileStepUpMethod
	TTL          time.Duration

	// 空なら usecase 側で生成
	ProvidedChallengeValue string
}

type RequestMobileStepUpOutput struct {
	Challenge           run.MobileStepUpChallenge
	PlainChallengeValue string
	ChallengeNonce      string
	ExpiresAt           time.Time
}

func (uc *RequestMobileStepUpUseCase) Handle(ctx context.Context, in RequestMobileStepUpInput) (RequestMobileStepUpOutput, error) {
	if uc.Devices == nil || uc.StepUps == nil {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: repositories are not fully configured")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: actor_user_id is required")
	}
	if strings.TrimSpace(in.DevicePublicID) == "" {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: device_public_id is required")
	}
	if in.ActionKind != run.MobileActionApprove &&
		in.ActionKind != run.MobileActionReject &&
		in.ActionKind != run.MobileActionAck {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: invalid action_kind")
	}

	traceID := ensureV17TraceID(in.TraceID)
	now := v17Now()
	ttl := normalizeV17TTL(in.TTL)

	device, err := loadV17ActiveOwnedDevice(ctx, uc.Devices, in.ProjectID, in.DevicePublicID, in.ActorUserID)
	if err != nil {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup load device: %w", err)
	}

	scopeKind := run.MobileStepUpScopeRunAction
	var targetInboxItemID *int64
	targetSourceType := strings.TrimSpace(in.TargetSourceType)
	targetSourceID := strings.TrimSpace(in.TargetSourceID)
	runID := strings.TrimSpace(in.RunID)

	if strings.TrimSpace(in.InboxItemPublicID) != "" {
		if uc.Inbox == nil {
			return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: inbox repository is nil")
		}
		item, err := uc.Inbox.FindByPublicID(ctx, in.ProjectID, in.InboxItemPublicID)
		if err != nil {
			return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup load inbox item: %w", err)
		}
		if item.IsTerminal() {
			return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: terminal_target")
		}
		scopeKind = run.MobileStepUpScopeInboxItem
		targetInboxItemID = &item.ID
		targetSourceType = item.SourceType
		targetSourceID = item.SourceID
		runID = item.RunID
	} else if targetSourceType != "" && targetSourceID != "" {
		scopeKind = run.MobileStepUpScopeSourceTarget
	} else if runID != "" {
		scopeKind = run.MobileStepUpScopeRunAction
	} else {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup: one of inbox_item_public_id, source_target, or run_id is required")
	}

	method := in.StepUpMethod
	if method == "" {
		method = run.MobileStepUpMethodOTP
	}

	challengeValue, nonce, err := buildV17ChallengeMaterial(method, in.ProvidedChallengeValue)
	if err != nil {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup build challenge material: %w", err)
	}

	var challengeCodeHash string
	if challengeValue != "" {
		challengeCodeHash = v17SHA256Hex(challengeValue)
	}

	challenge, err := uc.StepUps.Create(ctx, run.IssueMobileStepUpChallengeInput{
		PublicID:           newV17PublicID("mstep"),
		ProjectID:          in.ProjectID,
		ActorUserID:        in.ActorUserID,
		MobileDeviceID:     device.ID,
		ChallengeStatus:    run.MobileStepUpStatusIssued,
		StepUpMethod:       method,
		ChallengeCodeHash:  challengeCodeHash,
		ChallengeNonce:     nonce,
		ChallengeScopeKind: scopeKind,
		ActionKind:         in.ActionKind,
		TargetInboxItemID:  targetInboxItemID,
		TargetSourceType:   targetSourceType,
		TargetSourceID:     targetSourceID,
		RunID:              runID,
		TraceID:            traceID,
		IssuedAt:           now,
		ExpiresAt:          now.Add(ttl),
		VerifyAttemptCount: 0,
		MaxVerifyAttempts:  5,
		LastReasonCode:     "",
		IssuedByUserID:     in.ActorUserID,
	})
	if err != nil {
		return RequestMobileStepUpOutput{}, fmt.Errorf("request mobile stepup create: %w", err)
	}

	return RequestMobileStepUpOutput{
		Challenge:           challenge,
		PlainChallengeValue: challengeValue,
		ChallengeNonce:      nonce,
		ExpiresAt:           challenge.ExpiresAt,
	}, nil
}

func newV17PublicID(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func ensureV17TraceID(traceID string) string {
	traceID = strings.TrimSpace(traceID)
	if traceID != "" {
		return traceID
	}
	return "trace_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func v17Now() time.Time {
	return time.Now().UTC()
}

func normalizeV17TTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 5 * time.Minute
	}
	if ttl > 15*time.Minute {
		return 15 * time.Minute
	}
	return ttl
}

func v17SHA256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func buildV17ChallengeMaterial(method run.MobileStepUpMethod, provided string) (challengeValue string, nonce string, err error) {
	provided = strings.TrimSpace(provided)

	switch method {
	case run.MobileStepUpMethodOTP:
		if provided != "" {
			return provided, "", nil
		}
		code, err := randomV17Digits(6)
		if err != nil {
			return "", "", err
		}
		return code, "", nil

	case run.MobileStepUpMethodSignedNonce:
		if provided != "" {
			return provided, provided, nil
		}
		token := randomV17Token()
		return token, token, nil

	case run.MobileStepUpMethodWebAuthn, run.MobileStepUpMethodPlatformBiometric:
		// 現段階では nonce を返し、将来 verifier port で本格検証に置換
		token := randomV17Token()
		return "", token, nil

	case run.MobileStepUpMethodUnknown, "":
		return "", "", fmt.Errorf("unsupported stepup method")

	default:
		return "", "", fmt.Errorf("unsupported stepup method: %s", method)
	}
}

func randomV17Digits(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("random digits: invalid length")
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + v.Int64()))
	}
	return b.String(), nil
}

func randomV17Token() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func loadV17ActiveOwnedDevice(
	ctx context.Context,
	repo run.MobileDeviceRepository,
	projectID, devicePublicID, actorUserID string,
) (run.MobileDevice, error) {
	device, err := repo.FindByPublicID(ctx, projectID, devicePublicID)
	if err != nil {
		return run.MobileDevice{}, err
	}
	if device.ActorUserID != actorUserID {
		return run.MobileDevice{}, fmt.Errorf("device_actor_mismatch")
	}
	if !device.IsActive() {
		return run.MobileDevice{}, fmt.Errorf("device_not_active")
	}
	return device, nil
}

func bestEffortV17FindReceiptByIdempotency(
	ctx context.Context,
	repo run.MobileActionReceiptRepository,
	projectID, idempotencyKey string,
) (run.MobileActionReceipt, bool) {
	if repo == nil || strings.TrimSpace(idempotencyKey) == "" {
		return run.MobileActionReceipt{}, false
	}
	receipt, err := repo.FindByIdempotencyKey(ctx, run.FindMobileActionReceiptByIdempotencyInput{
		ProjectID:      projectID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return run.MobileActionReceipt{}, false
	}
	return receipt, true
}

func loadAndValidateV17VerifiedChallengeForInboxAction(
	ctx context.Context,
	stepUps run.MobileStepUpRepository,
	projectID, challengePublicID, actorUserID string,
	deviceID int64,
	actionKind run.MobileActionKind,
	item run.MobileInboxItem,
	traceID string,
	now time.Time,
) (run.MobileStepUpChallenge, error) {
	ch, err := stepUps.FindByPublicID(ctx, projectID, challengePublicID)
	if err != nil {
		return run.MobileStepUpChallenge{}, err
	}
	if ch.ActorUserID != actorUserID {
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_actor_mismatch")
	}
	if ch.MobileDeviceID != deviceID {
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_device_mismatch")
	}
	if ch.ActionKind != actionKind {
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_action_mismatch")
	}
	if ch.IsExpired(now) {
		_, _ = stepUps.MarkExpired(ctx, run.ExpireMobileStepUpChallengeInput{
			ProjectID:         projectID,
			ChallengePublicID: challengePublicID,
			ExpiredAt:         now,
			TraceID:           traceID,
			LastReasonCode:    "expired",
		})
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_expired")
	}
	if ch.ChallengeStatus == run.MobileStepUpStatusConsumed {
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_already_consumed")
	}
	if ch.ChallengeStatus != run.MobileStepUpStatusVerified {
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_not_verified")
	}

	switch ch.ChallengeScopeKind {
	case run.MobileStepUpScopeInboxItem:
		if ch.TargetInboxItemID == nil || *ch.TargetInboxItemID != item.ID {
			return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_scope_mismatch")
		}
	case run.MobileStepUpScopeSourceTarget:
		if ch.TargetSourceType != item.SourceType || ch.TargetSourceID != item.SourceID {
			return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_scope_mismatch")
		}
	case run.MobileStepUpScopeRunAction:
		if strings.TrimSpace(ch.RunID) == "" || strings.TrimSpace(ch.RunID) != strings.TrimSpace(item.RunID) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_scope_mismatch")
		}
	default:
		return run.MobileStepUpChallenge{}, fmt.Errorf("stepup_scope_mismatch")
	}

	return ch, nil
}
