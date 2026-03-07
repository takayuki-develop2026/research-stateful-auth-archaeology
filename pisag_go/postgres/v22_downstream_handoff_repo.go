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

type MultimodalDownstreamHandoffRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalDownstreamHandoffRepo(db *pgxpool.Pool) *MultimodalDownstreamHandoffRepo {
	return &MultimodalDownstreamHandoffRepo{db: db}
}

func (r *MultimodalDownstreamHandoffRepo) Create(ctx context.Context, in run.CreateMultimodalDownstreamHandoffInput) (run.MultimodalDownstreamHandoff, error) {
	const q = `
INSERT INTO multimodal_downstream_handoffs (
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	destination_kind,
	payload_evidence_asset_id,
	handoff_status,
	reason_code
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
	destination_kind,
	payload_evidence_asset_id,
	handoff_status,
	reason_code,
	created_at_utc,
	updated_at_utc,
	delivered_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.TaskID,
		in.ResultID,
		in.NormalizedResultID,
		in.DestinationKind,
		in.PayloadEvidenceAssetID,
		string(in.HandoffStatus),
		in.ReasonCode,
	)

	v, err := scanMultimodalDownstreamHandoff(row)
	if err != nil {
		return run.MultimodalDownstreamHandoff{}, fmt.Errorf("multimodal downstream handoff create: %w", err)
	}
	return v, nil
}

func (r *MultimodalDownstreamHandoffRepo) FindByID(ctx context.Context, id int64) (run.MultimodalDownstreamHandoff, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	destination_kind,
	payload_evidence_asset_id,
	handoff_status,
	reason_code,
	created_at_utc,
	updated_at_utc,
	delivered_at_utc
FROM multimodal_downstream_handoffs
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)
	v, err := scanMultimodalDownstreamHandoff(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalDownstreamHandoff{}, fmt.Errorf("multimodal downstream handoff not found: id=%d", id)
		}
		return run.MultimodalDownstreamHandoff{}, fmt.Errorf("multimodal downstream handoff find by id: %w", err)
	}
	return v, nil
}

func (r *MultimodalDownstreamHandoffRepo) ListByRunID(ctx context.Context, projectID, runID string) ([]run.MultimodalDownstreamHandoff, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_result_id,
	destination_kind,
	payload_evidence_asset_id,
	handoff_status,
	reason_code,
	created_at_utc,
	updated_at_utc,
	delivered_at_utc
FROM multimodal_downstream_handoffs
WHERE project_id = $1
  AND run_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("multimodal downstream handoff list by run id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalDownstreamHandoff
	for rows.Next() {
		v, err := scanMultimodalDownstreamHandoff(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal downstream handoff list by run id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal downstream handoff list by run id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalDownstreamHandoffRepo) MarkDelivered(ctx context.Context, projectID string, id int64, deliveredAtUTC time.Time) (run.MultimodalDownstreamHandoff, error) {
	const q = `
UPDATE multimodal_downstream_handoffs
SET
	handoff_status = 'delivered',
	delivered_at_utc = $3,
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
	destination_kind,
	payload_evidence_asset_id,
	handoff_status,
	reason_code,
	created_at_utc,
	updated_at_utc,
	delivered_at_utc
`
	row := r.db.QueryRow(ctx, q, projectID, id, deliveredAtUTC)
	v, err := scanMultimodalDownstreamHandoff(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalDownstreamHandoff{}, fmt.Errorf("multimodal downstream handoff mark delivered not found: project_id=%s id=%d", projectID, id)
		}
		return run.MultimodalDownstreamHandoff{}, fmt.Errorf("multimodal downstream handoff mark delivered: %w", err)
	}
	return v, nil
}

type multimodalDownstreamHandoffScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalDownstreamHandoff(s multimodalDownstreamHandoffScanner) (run.MultimodalDownstreamHandoff, error) {
	var out run.MultimodalDownstreamHandoff
	var status string
	var deliveredAt sql.NullTime

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.RunID,
		&out.TaskID,
		&out.ResultID,
		&out.NormalizedResultID,
		&out.DestinationKind,
		&out.PayloadEvidenceAssetID,
		&status,
		&out.ReasonCode,
		&out.CreatedAtUTC,
		&out.UpdatedAtUTC,
		&deliveredAt,
	)
	if err != nil {
		return run.MultimodalDownstreamHandoff{}, err
	}

	out.HandoffStatus = run.MultimodalDownstreamHandoffStatus(status)
	out.DeliveredAtUTC = nullTimePtrV22(deliveredAt)

	return out, nil
}
