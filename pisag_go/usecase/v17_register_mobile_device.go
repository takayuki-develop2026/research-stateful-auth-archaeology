package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterMobileDeviceUseCase struct {
	Devices run.MobileDeviceRepository
}

type RegisterMobileDeviceInput struct {
	ProjectID          string
	ActorUserID        string
	DeviceLabel        string
	PlatformType       run.MobilePlatformType
	AppChannel         run.MobileAppChannel
	DeviceFingerprint  string
	DeviceKeyID        string
	DevicePublicKeyPEM string
	AttestationFormat  string
	AttestationSubject string
	CreatedRunID       string
	TraceID            string

	ActivateImmediately bool
}

type RegisterMobileDeviceOutput struct {
	Device run.MobileDevice
}

func (uc *RegisterMobileDeviceUseCase) Handle(ctx context.Context, in RegisterMobileDeviceInput) (RegisterMobileDeviceOutput, error) {
	if uc.Devices == nil {
		return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device: devices repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device: project_id is required")
	}
	if strings.TrimSpace(in.ActorUserID) == "" {
		return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device: actor_user_id is required")
	}
	if strings.TrimSpace(in.DeviceFingerprint) == "" {
		return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device: device_fingerprint is required")
	}
	if in.PlatformType == "" {
		in.PlatformType = run.MobilePlatformUnknown
	}
	if in.AppChannel == "" {
		in.AppChannel = run.MobileAppChannelUnknown
	}

	traceID := ensureV17TraceID(in.TraceID)

	// 常に pending で作成し、必要なら直後に activate する。
	device, err := uc.Devices.Create(ctx, run.RegisterMobileDeviceInput{
		PublicID:           newV17PublicID("mdev"),
		ProjectID:          in.ProjectID,
		ActorUserID:        in.ActorUserID,
		DeviceStatus:       run.MobileDeviceStatusPending,
		DeviceLabel:        strings.TrimSpace(in.DeviceLabel),
		PlatformType:       in.PlatformType,
		AppChannel:         in.AppChannel,
		DeviceFingerprint:  strings.TrimSpace(in.DeviceFingerprint),
		DeviceKeyID:        strings.TrimSpace(in.DeviceKeyID),
		DevicePublicKeyPEM: strings.TrimSpace(in.DevicePublicKeyPEM),
		AttestationFormat:  strings.TrimSpace(in.AttestationFormat),
		AttestationSubject: strings.TrimSpace(in.AttestationSubject),
		CreatedRunID:       strings.TrimSpace(in.CreatedRunID),
		CreatedTraceID:     traceID,
		UpdatedTraceID:     traceID,
	})
	if err != nil {
		return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device create: %w", err)
	}

	if in.ActivateImmediately {
		device, err = uc.Devices.Activate(ctx, run.ActivateMobileDeviceInput{
			ProjectID:      in.ProjectID,
			DevicePublicID: device.PublicID,
			ActivatedAt:    v17Now(),
			UpdatedTraceID: traceID,
		})
		if err != nil {
			return RegisterMobileDeviceOutput{}, fmt.Errorf("register mobile device activate: %w", err)
		}
	}

	return RegisterMobileDeviceOutput{
		Device: device,
	}, nil
}
