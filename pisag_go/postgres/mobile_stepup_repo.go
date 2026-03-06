package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MobileStepUpRepo struct {
	db *pgxpool.Pool
}

func NewMobileStepUpRepo(db *pgxpool.Pool) *MobileStepUpRepo {
	return &MobileStepUpRepo{db: db}
}

func (r *MobileStepUpRepo) Create(ctx context.Context, in run.IssueMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
INSERT INTO mobile_stepup_challenges (
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
    $11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
)
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.PublicID,
		in.ProjectID,
		in.ActorUserID,
		in.MobileDeviceID,
		string(in.ChallengeStatus),
		string(in.StepUpMethod),
		nullableString2(in.ChallengeCodeHash),
		nullableString2(in.ChallengeNonce),
		string(in.ChallengeScopeKind),
		string(in.ActionKind),
		in.TargetInboxItemID,
		nullableString2(in.TargetSourceType),
		nullableString2(in.TargetSourceID),
		nullableString2(in.RunID),
		in.TraceID,
		in.IssuedAt,
		in.ExpiresAt,
		in.VerifyAttemptCount,
		in.MaxVerifyAttempts,
		nullableString2(in.LastReasonCode),
		nullableString2(in.IssuedByUserID),
	)

	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup create: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) FindByPublicID(ctx context.Context, projectID, publicID string) (run.MobileStepUpChallenge, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
FROM mobile_stepup_challenges
WHERE project_id = $1
  AND public_id = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, publicID)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup not found: project_id=%s public_id=%s", projectID, publicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup find by public id: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) FindOpenByScope(
	ctx context.Context,
	projectID, actorUserID string,
	deviceID int64,
	scopeKind run.MobileStepUpScopeKind,
	actionKind run.MobileActionKind,
	targetInboxItemID *int64,
	targetSourceType, targetSourceID, runID string,
) (run.MobileStepUpChallenge, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
FROM mobile_stepup_challenges
WHERE project_id = $1
  AND actor_user_id = $2
  AND mobile_device_id = $3
  AND challenge_scope_kind = $4
  AND action_kind = $5
  AND challenge_status IN ('issued', 'verified')
  AND (
        ($6::bigint IS NULL AND target_inbox_item_id IS NULL)
        OR target_inbox_item_id = $6
      )
  AND COALESCE(target_source_type, '') = COALESCE($7, '')
  AND COALESCE(target_source_id, '') = COALESCE($8, '')
  AND COALESCE(run_id, '') = COALESCE($9, '')
ORDER BY id DESC
LIMIT 1
`
	row := r.db.QueryRow(ctx, q,
		projectID,
		actorUserID,
		deviceID,
		string(scopeKind),
		string(actionKind),
		targetInboxItemID,
		nullableString2(targetSourceType),
		nullableString2(targetSourceID),
		nullableString2(runID),
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("open mobile stepup not found: project_id=%s actor_user_id=%s device_id=%d", projectID, actorUserID, deviceID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup find open by scope: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) IncrementVerifyAttempt(ctx context.Context, in run.IncrementStepUpVerifyAttemptInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    verify_attempt_count = verify_attempt_count + 1,
    last_reason_code = $5,
    trace_id = $6,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND actor_user_id = $3
  AND mobile_device_id = $4
  AND challenge_status IN ('issued', 'verified')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.ExpectedActorUserID,
		in.ExpectedDeviceID,
		nullableString2(in.LastReasonCode),
		in.TraceID,
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup increment verify attempt not found or not open: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup increment verify attempt: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) MarkVerified(ctx context.Context, in run.VerifyMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    challenge_status = 'verified',
    verified_at = $6,
    trace_id = $7,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND actor_user_id = $3
  AND mobile_device_id = $4
  AND action_kind = $5
  AND challenge_status = 'issued'
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.ExpectedActorUserID,
		in.ExpectedDeviceID,
		string(in.ExpectedActionKind),
		in.VerifiedAt,
		in.TraceID,
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup verify not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup mark verified: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) MarkConsumed(ctx context.Context, in run.ConsumeMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    challenge_status = 'consumed',
    consumed_at = $6,
    consumed_by_user_id = $7,
    last_reason_code = $8,
    trace_id = $9,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND actor_user_id = $3
  AND mobile_device_id = $4
  AND action_kind = $5
  AND challenge_status = 'verified'
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.ExpectedActorUserID,
		in.ExpectedDeviceID,
		string(in.ExpectedActionKind),
		in.ConsumedAt,
		in.ConsumedByUserID,
		nullableString2(in.LastReasonCode),
		in.TraceID,
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup consume not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup mark consumed: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) MarkExpired(ctx context.Context, in run.ExpireMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    challenge_status = 'expired',
    failed_at = $3,
    last_reason_code = $5,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND challenge_status IN ('issued', 'verified')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.ExpiredAt,
		in.TraceID,
		nullableString2(in.LastReasonCode),
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup expire not found or not open: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup mark expired: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) MarkRevoked(ctx context.Context, in run.RevokeMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    challenge_status = 'revoked',
    revoked_at = $3,
    last_reason_code = $5,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND challenge_status IN ('issued', 'verified')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.RevokedAt,
		in.TraceID,
		nullableString2(in.LastReasonCode),
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup revoke not found or not open: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup mark revoked: %w", err)
	}
	return ch, nil
}

func (r *MobileStepUpRepo) MarkFailed(ctx context.Context, in run.FailMobileStepUpChallengeInput) (run.MobileStepUpChallenge, error) {
	const q = `
UPDATE mobile_stepup_challenges
SET
    challenge_status = 'failed',
    failed_at = $3,
    last_reason_code = $5,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND challenge_status IN ('issued', 'verified')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    mobile_device_id,
    challenge_status,
    stepup_method,
    challenge_code_hash,
    challenge_nonce,
    challenge_scope_kind,
    action_kind,
    target_inbox_item_id,
    target_source_type,
    target_source_id,
    run_id,
    trace_id,
    issued_at,
    expires_at,
    verified_at,
    consumed_at,
    failed_at,
    revoked_at,
    verify_attempt_count,
    max_verify_attempts,
    last_reason_code,
    issued_by_user_id,
    consumed_by_user_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ChallengePublicID,
		in.FailedAt,
		in.TraceID,
		nullableString2(in.LastReasonCode),
	)
	ch, err := scanMobileStepUpChallenge(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup fail not found or not open: project_id=%s public_id=%s", in.ProjectID, in.ChallengePublicID)
		}
		return run.MobileStepUpChallenge{}, fmt.Errorf("mobile stepup mark failed: %w", err)
	}
	return ch, nil
}

type mobileStepUpScanner interface {
	Scan(dest ...any) error
}

func scanMobileStepUpChallenge(s mobileStepUpScanner) (run.MobileStepUpChallenge, error) {
	var out run.MobileStepUpChallenge

	var status string
	var method string
	var scopeKind string
	var actionKind string

	var challengeCodeHash sql.NullString
	var challengeNonce sql.NullString
	var targetInboxItemID sql.NullInt64
	var targetSourceType sql.NullString
	var targetSourceID sql.NullString
	var runID sql.NullString
	var verifiedAt sql.NullTime
	var consumedAt sql.NullTime
	var failedAt sql.NullTime
	var revokedAt sql.NullTime
	var lastReasonCode sql.NullString
	var issuedByUserID sql.NullString
	var consumedByUserID sql.NullString

	err := s.Scan(
		&out.ID,
		&out.PublicID,
		&out.ProjectID,
		&out.ActorUserID,
		&out.MobileDeviceID,
		&status,
		&method,
		&challengeCodeHash,
		&challengeNonce,
		&scopeKind,
		&actionKind,
		&targetInboxItemID,
		&targetSourceType,
		&targetSourceID,
		&runID,
		&out.TraceID,
		&out.IssuedAt,
		&out.ExpiresAt,
		&verifiedAt,
		&consumedAt,
		&failedAt,
		&revokedAt,
		&out.VerifyAttemptCount,
		&out.MaxVerifyAttempts,
		&lastReasonCode,
		&issuedByUserID,
		&consumedByUserID,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return run.MobileStepUpChallenge{}, err
	}

	out.ChallengeStatus = run.MobileStepUpStatus(status)
	out.StepUpMethod = run.MobileStepUpMethod(method)
	out.ChallengeCodeHash = nullStringValue2(challengeCodeHash)
	out.ChallengeNonce = nullStringValue2(challengeNonce)
	out.ChallengeScopeKind = run.MobileStepUpScopeKind(scopeKind)
	out.ActionKind = run.MobileActionKind(actionKind)
	out.TargetInboxItemID = nullInt64Ptr2(targetInboxItemID)
	out.TargetSourceType = nullStringValue2(targetSourceType)
	out.TargetSourceID = nullStringValue2(targetSourceID)
	out.RunID = nullStringValue2(runID)
	out.VerifiedAt = nullTimePtr2(verifiedAt)
	out.ConsumedAt = nullTimePtr2(consumedAt)
	out.FailedAt = nullTimePtr2(failedAt)
	out.RevokedAt = nullTimePtr2(revokedAt)
	out.LastReasonCode = nullStringValue2(lastReasonCode)
	out.IssuedByUserID = nullStringValue2(issuedByUserID)
	out.ConsumedByUserID = nullStringValue2(consumedByUserID)

	return out, nil
}

func nullableString2(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullStringValue2(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullTimePtr2(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullInt64Ptr2(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
