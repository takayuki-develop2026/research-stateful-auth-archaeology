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

type MobileDeviceRepo struct {
	db *pgxpool.Pool
}

func NewMobileDeviceRepo(db *pgxpool.Pool) *MobileDeviceRepo {
	return &MobileDeviceRepo{db: db}
}

func (r *MobileDeviceRepo) Create(ctx context.Context, in run.RegisterMobileDeviceInput) (run.MobileDevice, error) {
	const q = `
INSERT INTO mobile_devices (
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    created_run_id,
    created_trace_id,
    updated_trace_id
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
)
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.PublicID,
		in.ProjectID,
		in.ActorUserID,
		string(in.DeviceStatus),
		nullableString(in.DeviceLabel),
		string(in.PlatformType),
		string(in.AppChannel),
		in.DeviceFingerprint,
		nullableString(in.DeviceKeyID),
		nullableString(in.DevicePublicKeyPEM),
		nullableString(in.AttestationFormat),
		nullableString(in.AttestationSubject),
		nullableString(in.CreatedRunID),
		in.CreatedTraceID,
		in.UpdatedTraceID,
	)

	device, err := scanMobileDevice(row)
	if err != nil {
		return run.MobileDevice{}, fmt.Errorf("mobile device create: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) FindByPublicID(ctx context.Context, projectID, publicID string) (run.MobileDevice, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
FROM mobile_devices
WHERE project_id = $1
  AND public_id = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, publicID)
	device, err := scanMobileDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, fmt.Errorf("mobile device not found: project_id=%s public_id=%s", projectID, publicID)
		}
		return run.MobileDevice{}, fmt.Errorf("mobile device find by public id: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) FindActiveByFingerprint(ctx context.Context, projectID, actorUserID, fingerprint string) (run.MobileDevice, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
FROM mobile_devices
WHERE project_id = $1
  AND actor_user_id = $2
  AND device_fingerprint = $3
  AND device_status = 'active'
ORDER BY id DESC
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, actorUserID, fingerprint)
	device, err := scanMobileDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, fmt.Errorf("active mobile device not found: project_id=%s actor_user_id=%s fingerprint=%s", projectID, actorUserID, fingerprint)
		}
		return run.MobileDevice{}, fmt.Errorf("mobile device find active by fingerprint: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) ListByActor(ctx context.Context, projectID, actorUserID string) ([]run.MobileDevice, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
FROM mobile_devices
WHERE project_id = $1
  AND actor_user_id = $2
ORDER BY id DESC
`
	rows, err := r.db.Query(ctx, q, projectID, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("mobile device list by actor: %w", err)
	}
	defer rows.Close()

	var out []run.MobileDevice
	for rows.Next() {
		device, err := scanMobileDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("mobile device list by actor scan: %w", err)
		}
		out = append(out, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mobile device list by actor rows: %w", err)
	}
	return out, nil
}

func (r *MobileDeviceRepo) Activate(ctx context.Context, in run.ActivateMobileDeviceInput) (run.MobileDevice, error) {
	const q = `
UPDATE mobile_devices
SET
    device_status = 'active',
    activated_at = $3,
    updated_trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND device_status IN ('pending', 'disabled')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.DevicePublicID,
		in.ActivatedAt,
		in.UpdatedTraceID,
	)
	device, err := scanMobileDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, fmt.Errorf("mobile device activate not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.DevicePublicID)
		}
		return run.MobileDevice{}, fmt.Errorf("mobile device activate: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) UpdateLastSeen(ctx context.Context, in run.UpdateMobileDeviceLastSeenInput) error {
	const q = `
UPDATE mobile_devices
SET
    last_seen_at = $3,
    updated_trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
`
	tag, err := r.db.Exec(ctx, q,
		in.ProjectID,
		in.DevicePublicID,
		in.LastSeenAt,
		in.UpdatedTraceID,
	)
	if err != nil {
		return fmt.Errorf("mobile device update last seen: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mobile device update last seen not found: project_id=%s public_id=%s", in.ProjectID, in.DevicePublicID)
	}
	return nil
}

func (r *MobileDeviceRepo) Disable(ctx context.Context, in run.DisableMobileDeviceInput) (run.MobileDevice, error) {
	const q = `
UPDATE mobile_devices
SET
    device_status = 'disabled',
    disabled_at = $3,
    updated_trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND device_status IN ('pending', 'active')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.DevicePublicID,
		in.DisabledAt,
		in.UpdatedTraceID,
	)
	device, err := scanMobileDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, fmt.Errorf("mobile device disable not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.DevicePublicID)
		}
		return run.MobileDevice{}, fmt.Errorf("mobile device disable: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) Revoke(ctx context.Context, in run.RevokeMobileDeviceInput) (run.MobileDevice, error) {
	const q = `
UPDATE mobile_devices
SET
    device_status = 'revoked',
    revoked_at = $3,
    updated_trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND device_status IN ('pending', 'active', 'disabled')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.DevicePublicID,
		in.RevokedAt,
		in.UpdatedTraceID,
	)
	device, err := scanMobileDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, fmt.Errorf("mobile device revoke not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.DevicePublicID)
		}
		return run.MobileDevice{}, fmt.Errorf("mobile device revoke: %w", err)
	}
	return device, nil
}

func (r *MobileDeviceRepo) Rotate(ctx context.Context, in run.RotateMobileDeviceInput) (oldDevice run.MobileDevice, newDevice run.MobileDevice, err error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return run.MobileDevice{}, run.MobileDevice{}, fmt.Errorf("mobile device rotate begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
			return
		}
		err = tx.Commit(ctx)
	}()

	const qOld = `
UPDATE mobile_devices
SET
    device_status = 'rotated',
    updated_trace_id = $3,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND device_status IN ('active', 'disabled')
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	oldRow := tx.QueryRow(ctx, qOld, in.ProjectID, in.OldDevicePublicID, in.UpdatedTraceID)
	oldDevice, err = scanMobileDevice(oldRow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileDevice{}, run.MobileDevice{}, fmt.Errorf("mobile device rotate old device not found or not rotatable: project_id=%s public_id=%s", in.ProjectID, in.OldDevicePublicID)
		}
		return run.MobileDevice{}, run.MobileDevice{}, fmt.Errorf("mobile device rotate old scan: %w", err)
	}

	const qNew = `
INSERT INTO mobile_devices (
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    activated_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id
) VALUES (
    $1,$2,$3,'active',$4,$5,$6,$7,$8,$9,$10,$11,now(),$12,$13,$14,$15
)
RETURNING
    id,
    public_id,
    project_id,
    actor_user_id,
    device_status,
    device_label,
    platform_type,
    app_channel,
    device_fingerprint,
    device_key_id,
    device_public_key_pem,
    attestation_format,
    attestation_subject,
    registered_at,
    activated_at,
    last_seen_at,
    disabled_at,
    revoked_at,
    rotated_from_device_id,
    created_run_id,
    created_trace_id,
    updated_trace_id,
    created_at,
    updated_at
`
	newRow := tx.QueryRow(ctx, qNew,
		in.NewDevicePublicID,
		in.ProjectID,
		oldDevice.ActorUserID,
		nullableString(in.NewDeviceLabel),
		string(in.NewPlatformType),
		string(in.NewAppChannel),
		in.NewFingerprint,
		nullableString(in.NewDeviceKeyID),
		nullableString(in.NewDevicePublicKey),
		nullableString(in.NewAttestationFmt),
		nullableString(in.NewAttestationSubj),
		oldDevice.ID,
		nullableString(in.CreatedRunID),
		in.CreatedTraceID,
		in.UpdatedTraceID,
	)
	newDevice, err = scanMobileDevice(newRow)
	if err != nil {
		return run.MobileDevice{}, run.MobileDevice{}, fmt.Errorf("mobile device rotate new insert: %w", err)
	}

	const qFixOld = `
UPDATE mobile_devices
SET rotated_from_device_id = $3
WHERE project_id = $1
  AND public_id = $2
`
	if _, err = tx.Exec(ctx, qFixOld, in.ProjectID, in.OldDevicePublicID, oldDevice.RotatedFromDeviceID); err != nil {
		return run.MobileDevice{}, run.MobileDevice{}, fmt.Errorf("mobile device rotate fix old rotated ref: %w", err)
	}

	return oldDevice, newDevice, nil
}

type mobileDeviceScanner interface {
	Scan(dest ...any) error
}

func scanMobileDevice(s mobileDeviceScanner) (run.MobileDevice, error) {
	var out run.MobileDevice

	var deviceStatus string
	var platformType string
	var appChannel string

	var deviceLabel sql.NullString
	var deviceKeyID sql.NullString
	var devicePublicKeyPEM sql.NullString
	var attestationFormat sql.NullString
	var attestationSubject sql.NullString
	var activatedAt sql.NullTime
	var lastSeenAt sql.NullTime
	var disabledAt sql.NullTime
	var revokedAt sql.NullTime
	var rotatedFromDeviceID sql.NullInt64
	var createdRunID sql.NullString

	err := s.Scan(
		&out.ID,
		&out.PublicID,
		&out.ProjectID,
		&out.ActorUserID,
		&deviceStatus,
		&deviceLabel,
		&platformType,
		&appChannel,
		&out.DeviceFingerprint,
		&deviceKeyID,
		&devicePublicKeyPEM,
		&attestationFormat,
		&attestationSubject,
		&out.RegisteredAt,
		&activatedAt,
		&lastSeenAt,
		&disabledAt,
		&revokedAt,
		&rotatedFromDeviceID,
		&createdRunID,
		&out.CreatedTraceID,
		&out.UpdatedTraceID,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return run.MobileDevice{}, err
	}

	out.DeviceStatus = run.MobileDeviceStatus(deviceStatus)
	out.DeviceLabel = nullStringValue(deviceLabel)
	out.PlatformType = run.MobilePlatformType(platformType)
	out.AppChannel = run.MobileAppChannel(appChannel)
	out.DeviceKeyID = nullStringValue(deviceKeyID)
	out.DevicePublicKeyPEM = nullStringValue(devicePublicKeyPEM)
	out.AttestationFormat = nullStringValue(attestationFormat)
	out.AttestationSubject = nullStringValue(attestationSubject)
	out.ActivatedAt = nullTimePtr(activatedAt)
	out.LastSeenAt = nullTimePtr(lastSeenAt)
	out.DisabledAt = nullTimePtr(disabledAt)
	out.RevokedAt = nullTimePtr(revokedAt)
	out.RotatedFromDeviceID = nullInt64Ptr(rotatedFromDeviceID)
	out.CreatedRunID = nullStringValue(createdRunID)

	return out, nil
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
