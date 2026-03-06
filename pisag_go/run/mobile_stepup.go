package run

import "time"

type MobileStepUpStatus string

const (
	MobileStepUpStatusIssued   MobileStepUpStatus = "issued"
	MobileStepUpStatusVerified MobileStepUpStatus = "verified"
	MobileStepUpStatusConsumed MobileStepUpStatus = "consumed"
	MobileStepUpStatusExpired  MobileStepUpStatus = "expired"
	MobileStepUpStatusFailed   MobileStepUpStatus = "failed"
	MobileStepUpStatusRevoked  MobileStepUpStatus = "revoked"
)

type MobileStepUpMethod string

const (
	MobileStepUpMethodOTP               MobileStepUpMethod = "otp"
	MobileStepUpMethodSignedNonce       MobileStepUpMethod = "signed_nonce"
	MobileStepUpMethodWebAuthn          MobileStepUpMethod = "webauthn"
	MobileStepUpMethodPlatformBiometric MobileStepUpMethod = "platform_biometric"
	MobileStepUpMethodUnknown           MobileStepUpMethod = "unknown"
)

type MobileStepUpScopeKind string

const (
	MobileStepUpScopeInboxItem    MobileStepUpScopeKind = "inbox_item"
	MobileStepUpScopeSourceTarget MobileStepUpScopeKind = "source_target"
	MobileStepUpScopeRunAction    MobileStepUpScopeKind = "run_action"
)

type MobileActionKind string

const (
	MobileActionApprove MobileActionKind = "approve"
	MobileActionReject  MobileActionKind = "reject"
	MobileActionAck     MobileActionKind = "ack"
)

type MobileStepUpChallenge struct {
	ID                 int64
	PublicID           string
	ProjectID          string
	ActorUserID        string
	MobileDeviceID     int64
	ChallengeStatus    MobileStepUpStatus
	StepUpMethod       MobileStepUpMethod
	ChallengeCodeHash  string
	ChallengeNonce     string
	ChallengeScopeKind MobileStepUpScopeKind
	ActionKind         MobileActionKind
	TargetInboxItemID  *int64
	TargetSourceType   string
	TargetSourceID     string
	RunID              string
	TraceID            string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	VerifiedAt         *time.Time
	ConsumedAt         *time.Time
	FailedAt           *time.Time
	RevokedAt          *time.Time
	VerifyAttemptCount int
	MaxVerifyAttempts  int
	LastReasonCode     string
	IssuedByUserID     string
	ConsumedByUserID   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (c MobileStepUpChallenge) IsExpired(now time.Time) bool {
	return now.After(c.ExpiresAt)
}

func (c MobileStepUpChallenge) CanConsume(now time.Time) bool {
	return c.ChallengeStatus == MobileStepUpStatusVerified && !c.IsExpired(now)
}

type IssueMobileStepUpChallengeInput struct {
	PublicID           string
	ProjectID          string
	ActorUserID        string
	MobileDeviceID     int64
	ChallengeStatus    MobileStepUpStatus
	StepUpMethod       MobileStepUpMethod
	ChallengeCodeHash  string
	ChallengeNonce     string
	ChallengeScopeKind MobileStepUpScopeKind
	ActionKind         MobileActionKind
	TargetInboxItemID  *int64
	TargetSourceType   string
	TargetSourceID     string
	RunID              string
	TraceID            string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	VerifyAttemptCount int
	MaxVerifyAttempts  int
	LastReasonCode     string
	IssuedByUserID     string
}

type IncrementStepUpVerifyAttemptInput struct {
	ProjectID           string
	ChallengePublicID   string
	ExpectedActorUserID string
	ExpectedDeviceID    int64
	TraceID             string
	LastReasonCode      string
}

type VerifyMobileStepUpChallengeInput struct {
	ProjectID           string
	ChallengePublicID   string
	ExpectedActorUserID string
	ExpectedDeviceID    int64
	ExpectedActionKind  MobileActionKind
	VerifiedAt          time.Time
	TraceID             string
}

type ConsumeMobileStepUpChallengeInput struct {
	ProjectID           string
	ChallengePublicID   string
	ExpectedActorUserID string
	ExpectedDeviceID    int64
	ExpectedActionKind  MobileActionKind
	ConsumedByUserID    string
	ConsumedAt          time.Time
	TraceID             string
	LastReasonCode      string
}

type ExpireMobileStepUpChallengeInput struct {
	ProjectID         string
	ChallengePublicID string
	ExpiredAt         time.Time
	TraceID           string
	LastReasonCode    string
}

type RevokeMobileStepUpChallengeInput struct {
	ProjectID         string
	ChallengePublicID string
	RevokedAt         time.Time
	TraceID           string
	LastReasonCode    string
}

type FailMobileStepUpChallengeInput struct {
	ProjectID         string
	ChallengePublicID string
	FailedAt          time.Time
	TraceID           string
	LastReasonCode    string
}
