package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type VerifyMobileStepUpUseCase struct {
	Devices run.MobileDeviceRepository
	StepUps run.MobileStepUpRepository
}

type VerifyMobileStepUpInput struct {
	ProjectID         string
	ActorUserID       string
	DevicePublicID    string
	ChallengePublicID string
	ActionKind        run.MobileActionKind
	VerificationValue string
	TraceID           string
}

type VerifyMobileStepUpOutput struct {
	Challenge run.MobileStepUpChallenge
}

func (uc *VerifyMobileStepUpUseCase) Handle(ctx context.Context, in VerifyMobileStepUpInput) (VerifyMobileStepUpOutput, error) {
	if uc.Devices == nil || uc.StepUps == nil {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: repositories are not fully configured")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: actor_user_id is required")
	}
	if strings.TrimSpace(in.DevicePublicID) == "" {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: device_public_id is required")
	}
	if strings.TrimSpace(in.ChallengePublicID) == "" {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: challenge_public_id is required")
	}
	if in.ActionKind == "" {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: action_kind is required")
	}

	traceID := ensureV17TraceID(in.TraceID)
	now := v17Now()

	device, err := loadV17ActiveOwnedDevice(ctx, uc.Devices, in.ProjectID, in.DevicePublicID, in.ActorUserID)
	if err != nil {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup load device: %w", err)
	}

	challenge, err := uc.StepUps.FindByPublicID(ctx, in.ProjectID, in.ChallengePublicID)
	if err != nil {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup load challenge: %w", err)
	}
	if challenge.ActorUserID != in.ActorUserID {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: actor_mismatch")
	}
	if challenge.MobileDeviceID != device.ID {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: device_mismatch")
	}
	if challenge.ActionKind != in.ActionKind {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: action_mismatch")
	}

	if challenge.IsExpired(now) {
		_, _ = uc.StepUps.MarkExpired(ctx, run.ExpireMobileStepUpChallengeInput{
			ProjectID:         in.ProjectID,
			ChallengePublicID: in.ChallengePublicID,
			ExpiredAt:         now,
			TraceID:           traceID,
			LastReasonCode:    "expired",
		})
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: expired")
	}

	switch challenge.ChallengeStatus {
	case run.MobileStepUpStatusVerified:
		return VerifyMobileStepUpOutput{Challenge: challenge}, nil
	case run.MobileStepUpStatusConsumed:
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: already_consumed")
	case run.MobileStepUpStatusExpired:
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: expired")
	case run.MobileStepUpStatusFailed:
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: failed")
	case run.MobileStepUpStatusRevoked:
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: revoked")
	case run.MobileStepUpStatusIssued:
		// continue
	default:
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: invalid status")
	}

	ok := false
	verifyValue := strings.TrimSpace(in.VerificationValue)

	switch challenge.StepUpMethod {
	case run.MobileStepUpMethodOTP:
		ok = verifyValue != "" && v17SHA256Hex(verifyValue) == challenge.ChallengeCodeHash

	case run.MobileStepUpMethodSignedNonce:
		ok = verifyValue != "" && verifyValue == challenge.ChallengeNonce

	case run.MobileStepUpMethodWebAuthn, run.MobileStepUpMethodPlatformBiometric:
		// 将来は verifier port に差し替え。
		// 現段階では nonce の一致のみで簡易確認しない。
		ok = false

	default:
		ok = false
	}

	if !ok {
		updated, incErr := uc.StepUps.IncrementVerifyAttempt(ctx, run.IncrementStepUpVerifyAttemptInput{
			ProjectID:           in.ProjectID,
			ChallengePublicID:   in.ChallengePublicID,
			ExpectedActorUserID: in.ActorUserID,
			ExpectedDeviceID:    device.ID,
			TraceID:             traceID,
			LastReasonCode:      "invalid_code",
		})
		if incErr != nil {
			return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup increment attempt: %w", incErr)
		}
		if updated.VerifyAttemptCount >= updated.MaxVerifyAttempts {
			_, _ = uc.StepUps.MarkFailed(ctx, run.FailMobileStepUpChallengeInput{
				ProjectID:         in.ProjectID,
				ChallengePublicID: in.ChallengePublicID,
				FailedAt:          now,
				TraceID:           traceID,
				LastReasonCode:    "max_attempts_exceeded",
			})
		}
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup: invalid_verification")
	}

	verified, err := uc.StepUps.MarkVerified(ctx, run.VerifyMobileStepUpChallengeInput{
		ProjectID:           in.ProjectID,
		ChallengePublicID:   in.ChallengePublicID,
		ExpectedActorUserID: in.ActorUserID,
		ExpectedDeviceID:    device.ID,
		ExpectedActionKind:  in.ActionKind,
		VerifiedAt:          now,
		TraceID:             traceID,
	})
	if err != nil {
		return VerifyMobileStepUpOutput{}, fmt.Errorf("verify mobile stepup mark verified: %w", err)
	}

	return VerifyMobileStepUpOutput{
		Challenge: verified,
	}, nil
}
