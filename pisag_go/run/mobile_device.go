package run

import "time"

type MobileDeviceStatus string

const (
	MobileDeviceStatusPending  MobileDeviceStatus = "pending"
	MobileDeviceStatusActive   MobileDeviceStatus = "active"
	MobileDeviceStatusDisabled MobileDeviceStatus = "disabled"
	MobileDeviceStatusRevoked  MobileDeviceStatus = "revoked"
	MobileDeviceStatusRotated  MobileDeviceStatus = "rotated"
)

type MobilePlatformType string

const (
	MobilePlatformIOS       MobilePlatformType = "ios"
	MobilePlatformAndroid   MobilePlatformType = "android"
	MobilePlatformWebMobile MobilePlatformType = "web_mobile"
	MobilePlatformUnknown   MobilePlatformType = "unknown"
)

type MobileAppChannel string

const (
	MobileAppChannelPWA     MobileAppChannel = "pwa"
	MobileAppChannelNative  MobileAppChannel = "native"
	MobileAppChannelBrowser MobileAppChannel = "browser"
	MobileAppChannelUnknown MobileAppChannel = "unknown"
)

type MobileDevice struct {
	ID                  int64
	PublicID            string
	ProjectID           string
	ActorUserID         string
	DeviceStatus        MobileDeviceStatus
	DeviceLabel         string
	PlatformType        MobilePlatformType
	AppChannel          MobileAppChannel
	DeviceFingerprint   string
	DeviceKeyID         string
	DevicePublicKeyPEM  string
	AttestationFormat   string
	AttestationSubject  string
	RegisteredAt        time.Time
	ActivatedAt         *time.Time
	LastSeenAt          *time.Time
	DisabledAt          *time.Time
	RevokedAt           *time.Time
	RotatedFromDeviceID *int64
	CreatedRunID        string
	CreatedTraceID      string
	UpdatedTraceID      string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (d MobileDevice) IsActive() bool {
	return d.DeviceStatus == MobileDeviceStatusActive
}

func (d MobileDevice) IsTerminal() bool {
	return d.DeviceStatus == MobileDeviceStatusRevoked ||
		d.DeviceStatus == MobileDeviceStatusRotated
}

type RegisterMobileDeviceInput struct {
	PublicID           string
	ProjectID          string
	ActorUserID        string
	DeviceStatus       MobileDeviceStatus
	DeviceLabel        string
	PlatformType       MobilePlatformType
	AppChannel         MobileAppChannel
	DeviceFingerprint  string
	DeviceKeyID        string
	DevicePublicKeyPEM string
	AttestationFormat  string
	AttestationSubject string
	CreatedRunID       string
	CreatedTraceID     string
	UpdatedTraceID     string
}

type UpdateMobileDeviceLastSeenInput struct {
	ProjectID      string
	DevicePublicID string
	LastSeenAt     time.Time
	UpdatedTraceID string
}

type ActivateMobileDeviceInput struct {
	ProjectID      string
	DevicePublicID string
	ActivatedAt    time.Time
	UpdatedTraceID string
}

type DisableMobileDeviceInput struct {
	ProjectID      string
	DevicePublicID string
	DisabledAt     time.Time
	UpdatedTraceID string
}

type RevokeMobileDeviceInput struct {
	ProjectID      string
	DevicePublicID string
	RevokedAt      time.Time
	UpdatedTraceID string
}

type RotateMobileDeviceInput struct {
	ProjectID          string
	OldDevicePublicID  string
	NewDevicePublicID  string
	NewDeviceLabel     string
	NewPlatformType    MobilePlatformType
	NewAppChannel      MobileAppChannel
	NewFingerprint     string
	NewDeviceKeyID     string
	NewDevicePublicKey string
	NewAttestationFmt  string
	NewAttestationSubj string
	CreatedRunID       string
	CreatedTraceID     string
	UpdatedTraceID     string
}
