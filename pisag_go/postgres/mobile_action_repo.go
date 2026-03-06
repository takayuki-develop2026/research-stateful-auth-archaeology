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

type MobileActionReceiptRepo struct {
	db *pgxpool.Pool
}

func NewMobileActionReceiptRepo(db *pgxpool.Pool) *MobileActionReceiptRepo {
	return &MobileActionReceiptRepo{db: db}
}

func (r *MobileActionReceiptRepo) Create(ctx context.Context, in run.CreateMobileActionReceiptInput) (run.MobileActionReceipt, error) {
	const q = `
INSERT INTO mobile_action_receipts (
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
)
RETURNING
    id,
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.PublicID,
		in.ProjectID,
		string(in.ActionKind),
		string(in.OutcomeStatus),
		nullableString4(in.ReasonCode),
		in.MobileInboxItemID,
		in.MobileDeviceID,
		in.MobileStepUpChallengeID,
		in.ActorUserID,
		in.SourceType,
		in.SourceID,
		nullableString4(in.RunID),
		in.TraceID,
		nullableString4(in.IdempotencyKey),
		nullableString4(in.CommentText),
		in.AttemptedAt,
		in.CompletedAt,
	)

	receipt, err := scanMobileActionReceipt(row)
	if err != nil {
		return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt create: %w", err)
	}
	return receipt, nil
}

func (r *MobileActionReceiptRepo) FindByPublicID(ctx context.Context, projectID, publicID string) (run.MobileActionReceipt, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at,
    created_at,
    updated_at
FROM mobile_action_receipts
WHERE project_id = $1
  AND public_id = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, publicID)
	receipt, err := scanMobileActionReceipt(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt not found: project_id=%s public_id=%s", projectID, publicID)
		}
		return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt find by public id: %w", err)
	}
	return receipt, nil
}

func (r *MobileActionReceiptRepo) FindByIdempotencyKey(ctx context.Context, in run.FindMobileActionReceiptByIdempotencyInput) (run.MobileActionReceipt, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at,
    created_at,
    updated_at
FROM mobile_action_receipts
WHERE project_id = $1
  AND idempotency_key = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.IdempotencyKey)
	receipt, err := scanMobileActionReceipt(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt by idempotency not found: project_id=%s idempotency_key=%s", in.ProjectID, in.IdempotencyKey)
		}
		return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt find by idempotency: %w", err)
	}
	return receipt, nil
}

func (r *MobileActionReceiptRepo) ListByInboxItem(ctx context.Context, projectID string, inboxItemID int64) ([]run.MobileActionReceipt, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at,
    created_at,
    updated_at
FROM mobile_action_receipts
WHERE project_id = $1
  AND mobile_inbox_item_id = $2
ORDER BY id DESC
`
	rows, err := r.db.Query(ctx, q, projectID, inboxItemID)
	if err != nil {
		return nil, fmt.Errorf("mobile action receipt list by inbox item: %w", err)
	}
	defer rows.Close()

	var out []run.MobileActionReceipt
	for rows.Next() {
		receipt, err := scanMobileActionReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("mobile action receipt list by inbox item scan: %w", err)
		}
		out = append(out, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mobile action receipt list by inbox item rows: %w", err)
	}
	return out, nil
}

func (r *MobileActionReceiptRepo) Complete(ctx context.Context, in run.CompleteMobileActionReceiptInput) (run.MobileActionReceipt, error) {
	const q = `
UPDATE mobile_action_receipts
SET
    outcome_status = $3,
    reason_code = $4,
    completed_at = $5,
    trace_id = $6,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
RETURNING
    id,
    public_id,
    project_id,
    action_kind,
    outcome_status,
    reason_code,
    mobile_inbox_item_id,
    mobile_device_id,
    mobile_stepup_challenge_id,
    actor_user_id,
    source_type,
    source_id,
    run_id,
    trace_id,
    idempotency_key,
    comment_text,
    attempted_at,
    completed_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ReceiptPublicID,
		string(in.OutcomeStatus),
		nullableString4(in.ReasonCode),
		in.CompletedAt,
		in.TraceID,
	)

	receipt, err := scanMobileActionReceipt(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt complete not found: project_id=%s public_id=%s", in.ProjectID, in.ReceiptPublicID)
		}
		return run.MobileActionReceipt{}, fmt.Errorf("mobile action receipt complete: %w", err)
	}
	return receipt, nil
}

type mobileActionReceiptScanner interface {
	Scan(dest ...any) error
}

func scanMobileActionReceipt(s mobileActionReceiptScanner) (run.MobileActionReceipt, error) {
	var out run.MobileActionReceipt

	var actionKind string
	var outcomeStatus string
	var reasonCode sql.NullString
	var stepupChallengeID sql.NullInt64
	var runID sql.NullString
	var idempotencyKey sql.NullString
	var commentText sql.NullString
	var completedAt sql.NullTime

	err := s.Scan(
		&out.ID,
		&out.PublicID,
		&out.ProjectID,
		&actionKind,
		&outcomeStatus,
		&reasonCode,
		&out.MobileInboxItemID,
		&out.MobileDeviceID,
		&stepupChallengeID,
		&out.ActorUserID,
		&out.SourceType,
		&out.SourceID,
		&runID,
		&out.TraceID,
		&idempotencyKey,
		&commentText,
		&out.AttemptedAt,
		&completedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return run.MobileActionReceipt{}, err
	}

	out.ActionKind = run.MobileActionKind(actionKind)
	out.OutcomeStatus = run.MobileActionOutcomeStatus(outcomeStatus)
	out.ReasonCode = nullStringValue4(reasonCode)
	out.MobileStepUpChallengeID = nullInt64Ptr4(stepupChallengeID)
	out.RunID = nullStringValue4(runID)
	out.IdempotencyKey = nullStringValue4(idempotencyKey)
	out.CommentText = nullStringValue4(commentText)
	out.CompletedAt = nullTimePtr4(completedAt)

	return out, nil
}

func nullableString4(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullStringValue4(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullTimePtr4(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullInt64Ptr4(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
