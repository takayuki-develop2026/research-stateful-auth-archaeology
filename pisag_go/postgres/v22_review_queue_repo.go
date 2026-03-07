package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	run "example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MultimodalReviewQueueRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalReviewQueueRepo(db *pgxpool.Pool) *MultimodalReviewQueueRepo {
	return &MultimodalReviewQueueRepo{db: db}
}

func (r *MultimodalReviewQueueRepo) Create(ctx context.Context, in run.CreateMultimodalReviewQueueItemInput) (run.MultimodalReviewQueueItem, error) {
	const q = `
INSERT INTO multimodal_review_queue (
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10
)
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id,
	created_at_utc,
	updated_at_utc,
	resolved_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.TaskID,
		in.ResultID,
		in.NormalizedResultID,
		string(in.QueueStatus),
		string(in.Priority),
		in.ReasonCode,
		in.AssignedReviewerID,
	)

	v, err := scanMultimodalReviewQueueItem(row)
	if err != nil {
		return run.MultimodalReviewQueueItem{}, fmt.Errorf("multimodal review queue create: %w", err)
	}
	return v, nil
}

func (r *MultimodalReviewQueueRepo) FindByID(ctx context.Context, id int64) (run.MultimodalReviewQueueItem, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id,
	created_at_utc,
	updated_at_utc,
	resolved_at_utc
FROM multimodal_review_queue
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)
	v, err := scanMultimodalReviewQueueItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalReviewQueueItem{}, fmt.Errorf("multimodal review queue not found: id=%d", id)
		}
		return run.MultimodalReviewQueueItem{}, fmt.Errorf("multimodal review queue find by id: %w", err)
	}
	return v, nil
}

func (r *MultimodalReviewQueueRepo) ListByRunID(ctx context.Context, projectID, runID string) ([]run.MultimodalReviewQueueItem, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id,
	created_at_utc,
	updated_at_utc,
	resolved_at_utc
FROM multimodal_review_queue
WHERE project_id = $1
  AND run_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("multimodal review queue list by run id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalReviewQueueItem
	for rows.Next() {
		v, err := scanMultimodalReviewQueueItem(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal review queue list by run id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal review queue list by run id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalReviewQueueRepo) ListPendingByProjectID(ctx context.Context, projectID string) ([]run.MultimodalReviewQueueItem, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id,
	created_at_utc,
	updated_at_utc,
	resolved_at_utc
FROM multimodal_review_queue
WHERE project_id = $1
  AND queue_status = 'pending'
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("multimodal review queue list pending by project id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalReviewQueueItem
	for rows.Next() {
		v, err := scanMultimodalReviewQueueItem(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal review queue list pending by project id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal review queue list pending by project id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalReviewQueueRepo) MarkResolved(ctx context.Context, projectID string, id int64, resolvedAtUTC time.Time) (run.MultimodalReviewQueueItem, error) {
	const q = `
UPDATE multimodal_review_queue
SET
	queue_status = 'resolved',
	resolved_at_utc = $3,
	updated_at_utc = now()
WHERE project_id = $1
  AND id = $2
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	queue_status,
	priority,
	reason_code,
	assigned_reviewer_id,
	created_at_utc,
	updated_at_utc,
	resolved_at_utc
`
	row := r.db.QueryRow(ctx, q, projectID, id, resolvedAtUTC)
	v, err := scanMultimodalReviewQueueItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalReviewQueueItem{}, fmt.Errorf("multimodal review queue resolve not found: project_id=%s id=%d", projectID, id)
		}
		return run.MultimodalReviewQueueItem{}, fmt.Errorf("multimodal review queue resolve: %w", err)
	}
	return v, nil
}

type multimodalReviewQueueScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalReviewQueueItem(s multimodalReviewQueueScanner) (run.MultimodalReviewQueueItem, error) {
	var out run.MultimodalReviewQueueItem
	var status string
	var priority string
	var resolvedAt sql.NullTime

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.RunID,
		&out.TaskID,
		&out.ResultID,
		&out.NormalizedResultID,
		&status,
		&priority,
		&out.ReasonCode,
		&out.AssignedReviewerID,
		&out.CreatedAtUTC,
		&out.UpdatedAtUTC,
		&resolvedAt,
	)
	if err != nil {
		return run.MultimodalReviewQueueItem{}, err
	}

	out.QueueStatus = run.MultimodalReviewQueueStatus(status)
	out.Priority = run.MultimodalReviewQueuePriority(priority)
	out.ResolvedAtUTC = nullTimePtrV22(resolvedAt)

	return out, nil
}
