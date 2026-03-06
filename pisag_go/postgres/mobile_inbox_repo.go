package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MobileInboxRepo struct {
	db *pgxpool.Pool
}

func NewMobileInboxRepo(db *pgxpool.Pool) *MobileInboxRepo {
	return &MobileInboxRepo{db: db}
}

func (r *MobileInboxRepo) Create(ctx context.Context, in run.CreateMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
INSERT INTO mobile_inbox_items (
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    due_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
    $13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24
)
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q,
		in.PublicID,
		in.ProjectID,
		string(in.InboxStatus),
		string(in.ItemKind),
		in.SourceType,
		in.SourceID,
		nullableString3(in.RunID),
		in.TraceID,
		nullableString3(in.ActorUserID),
		nullableString3(in.AssignedUserID),
		string(in.Priority),
		string(in.Severity),
		in.Title,
		nullableString3(in.Summary),
		in.ActionRequired,
		in.StepUpRequired,
		in.CommentRequired,
		in.AvailableActionApprove,
		in.AvailableActionReject,
		in.AvailableActionAck,
		nullableString3(in.TerminalReasonCode),
		in.SourceOccurredAt,
		in.FirstPresentedAt,
		in.DueAt,
	)

	item, err := scanMobileInboxItem(row)
	if err != nil {
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox create: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) FindByPublicID(ctx context.Context, projectID, publicID string) (run.MobileInboxItem, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
FROM mobile_inbox_items
WHERE project_id = $1
  AND public_id = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, publicID)
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox not found: project_id=%s public_id=%s", projectID, publicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox find by public id: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) FindOpenBySource(ctx context.Context, projectID, sourceType, sourceID string) (run.MobileInboxItem, error) {
	const q = `
SELECT
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
FROM mobile_inbox_items
WHERE project_id = $1
  AND source_type = $2
  AND source_id = $3
  AND inbox_status IN ('pending', 'acknowledged')
ORDER BY id DESC
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, sourceType, sourceID)
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("open mobile inbox by source not found: project_id=%s source_type=%s source_id=%s", projectID, sourceType, sourceID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox find open by source: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) List(ctx context.Context, filter run.ListMobileInboxItemsFilter) ([]run.MobileInboxItem, error) {
	var sb strings.Builder
	sb.WriteString(`
SELECT
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
FROM mobile_inbox_items
WHERE project_id = $1
`)
	args := []any{filter.ProjectID}
	argPos := 2

	if filter.AssignedUserID != "" {
		sb.WriteString(fmt.Sprintf(" AND assigned_user_id = $%d", argPos))
		args = append(args, filter.AssignedUserID)
		argPos++
	}
	if filter.ActorUserID != "" {
		sb.WriteString(fmt.Sprintf(" AND actor_user_id = $%d", argPos))
		args = append(args, filter.ActorUserID)
		argPos++
	}
	if filter.Status != "" {
		sb.WriteString(fmt.Sprintf(" AND inbox_status = $%d", argPos))
		args = append(args, string(filter.Status))
		argPos++
	}
	if filter.ItemKind != "" {
		sb.WriteString(fmt.Sprintf(" AND item_kind = $%d", argPos))
		args = append(args, string(filter.ItemKind))
		argPos++
	}
	if filter.Priority != "" {
		sb.WriteString(fmt.Sprintf(" AND priority = $%d", argPos))
		args = append(args, string(filter.Priority))
		argPos++
	}
	if filter.Severity != "" {
		sb.WriteString(fmt.Sprintf(" AND severity = $%d", argPos))
		args = append(args, string(filter.Severity))
		argPos++
	}
	if filter.OnlyActionable {
		sb.WriteString(` AND action_required = true AND inbox_status IN ('pending', 'acknowledged')`)
	}

	sb.WriteString(`
ORDER BY
    CASE priority
        WHEN 'urgent' THEN 1
        WHEN 'high' THEN 2
        WHEN 'normal' THEN 3
        WHEN 'low' THEN 4
        ELSE 9
    END,
    COALESCE(due_at, created_at) ASC,
    id DESC
`)

	if filter.Limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT $%d", argPos))
		args = append(args, filter.Limit)
		argPos++
	}
	if filter.Offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET $%d", argPos))
		args = append(args, filter.Offset)
		argPos++
	}

	rows, err := r.db.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("mobile inbox list: %w", err)
	}
	defer rows.Close()

	var out []run.MobileInboxItem
	for rows.Next() {
		item, err := scanMobileInboxItem(rows)
		if err != nil {
			return nil, fmt.Errorf("mobile inbox list scan: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mobile inbox list rows: %w", err)
	}
	return out, nil
}

func (r *MobileInboxRepo) MarkAcknowledged(ctx context.Context, in run.AcknowledgeMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'acknowledged',
    first_acknowledged_at = COALESCE(first_acknowledged_at, $3),
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status = 'pending'
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.AcknowledgedAt, in.TraceID)
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox acknowledge not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark acknowledged: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) MarkApproved(ctx context.Context, in run.ApproveMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'approved',
    approved_at = $3,
    terminal_at = $3,
    terminal_reason_code = $5,
    action_required = false,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status IN ('pending', 'acknowledged')
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.ApprovedAt, in.TraceID, nullableString3(in.TerminalReasonCode))
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox approve not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark approved: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) MarkRejected(ctx context.Context, in run.RejectMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'rejected',
    rejected_at = $3,
    terminal_at = $3,
    terminal_reason_code = $5,
    action_required = false,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status IN ('pending', 'acknowledged')
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.RejectedAt, in.TraceID, nullableString3(in.TerminalReasonCode))
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox reject not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark rejected: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) MarkExpired(ctx context.Context, in run.ExpireMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'expired',
    expired_at = $3,
    terminal_at = $3,
    terminal_reason_code = $5,
    action_required = false,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status IN ('pending', 'acknowledged')
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.ExpiredAt, in.TraceID, nullableString3(in.TerminalReasonCode))
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox expire not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark expired: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) MarkCanceled(ctx context.Context, in run.CancelMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'canceled',
    canceled_at = $3,
    terminal_at = $3,
    terminal_reason_code = $5,
    action_required = false,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status IN ('pending', 'acknowledged')
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.CanceledAt, in.TraceID, nullableString3(in.TerminalReasonCode))
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox cancel not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark canceled: %w", err)
	}
	return item, nil
}

func (r *MobileInboxRepo) MarkSuperseded(ctx context.Context, in run.SupersedeMobileInboxItemInput) (run.MobileInboxItem, error) {
	const q = `
UPDATE mobile_inbox_items
SET
    inbox_status = 'superseded',
    superseded_at = $3,
    terminal_at = $3,
    terminal_reason_code = $5,
    action_required = false,
    trace_id = $4,
    updated_at = now()
WHERE project_id = $1
  AND public_id = $2
  AND inbox_status IN ('pending', 'acknowledged')
RETURNING
    id,
    public_id,
    project_id,
    inbox_status,
    item_kind,
    source_type,
    source_id,
    run_id,
    trace_id,
    actor_user_id,
    assigned_user_id,
    priority,
    severity,
    title,
    summary,
    action_required,
    stepup_required,
    comment_required,
    available_action_approve,
    available_action_reject,
    available_action_ack,
    terminal_at,
    terminal_reason_code,
    source_occurred_at,
    first_presented_at,
    first_acknowledged_at,
    approved_at,
    rejected_at,
    expired_at,
    canceled_at,
    superseded_at,
    due_at,
    created_at,
    updated_at
`
	row := r.db.QueryRow(ctx, q, in.ProjectID, in.InboxItemPublicID, in.SupersededAt, in.TraceID, nullableString3(in.TerminalReasonCode))
	item, err := scanMobileInboxItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MobileInboxItem{}, fmt.Errorf("mobile inbox supersede not allowed or not found: project_id=%s public_id=%s", in.ProjectID, in.InboxItemPublicID)
		}
		return run.MobileInboxItem{}, fmt.Errorf("mobile inbox mark superseded: %w", err)
	}
	return item, nil
}

type mobileInboxScanner interface {
	Scan(dest ...any) error
}

func scanMobileInboxItem(s mobileInboxScanner) (run.MobileInboxItem, error) {
	var out run.MobileInboxItem

	var inboxStatus string
	var itemKind string
	var runID sql.NullString
	var actorUserID sql.NullString
	var assignedUserID sql.NullString
	var priority string
	var severity string
	var summary sql.NullString
	var terminalAt sql.NullTime
	var terminalReasonCode sql.NullString
	var sourceOccurredAt sql.NullTime
	var firstPresentedAt sql.NullTime
	var firstAcknowledgedAt sql.NullTime
	var approvedAt sql.NullTime
	var rejectedAt sql.NullTime
	var expiredAt sql.NullTime
	var canceledAt sql.NullTime
	var supersededAt sql.NullTime
	var dueAt sql.NullTime

	err := s.Scan(
		&out.ID,
		&out.PublicID,
		&out.ProjectID,
		&inboxStatus,
		&itemKind,
		&out.SourceType,
		&out.SourceID,
		&runID,
		&out.TraceID,
		&actorUserID,
		&assignedUserID,
		&priority,
		&severity,
		&out.Title,
		&summary,
		&out.ActionRequired,
		&out.StepUpRequired,
		&out.CommentRequired,
		&out.AvailableActionApprove,
		&out.AvailableActionReject,
		&out.AvailableActionAck,
		&terminalAt,
		&terminalReasonCode,
		&sourceOccurredAt,
		&firstPresentedAt,
		&firstAcknowledgedAt,
		&approvedAt,
		&rejectedAt,
		&expiredAt,
		&canceledAt,
		&supersededAt,
		&dueAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return run.MobileInboxItem{}, err
	}

	out.InboxStatus = run.MobileInboxStatus(inboxStatus)
	out.ItemKind = run.MobileInboxItemKind(itemKind)
	out.RunID = nullStringValue3(runID)
	out.ActorUserID = nullStringValue3(actorUserID)
	out.AssignedUserID = nullStringValue3(assignedUserID)
	out.Priority = run.MobilePriority(priority)
	out.Severity = run.MobileSeverity(severity)
	out.Summary = nullStringValue3(summary)
	out.TerminalAt = nullTimePtr3(terminalAt)
	out.TerminalReasonCode = nullStringValue3(terminalReasonCode)
	out.SourceOccurredAt = nullTimePtr3(sourceOccurredAt)
	out.FirstPresentedAt = nullTimePtr3(firstPresentedAt)
	out.FirstAcknowledgedAt = nullTimePtr3(firstAcknowledgedAt)
	out.ApprovedAt = nullTimePtr3(approvedAt)
	out.RejectedAt = nullTimePtr3(rejectedAt)
	out.ExpiredAt = nullTimePtr3(expiredAt)
	out.CanceledAt = nullTimePtr3(canceledAt)
	out.SupersededAt = nullTimePtr3(supersededAt)
	out.DueAt = nullTimePtr3(dueAt)

	return out, nil
}

func nullableString3(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullStringValue3(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullTimePtr3(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
