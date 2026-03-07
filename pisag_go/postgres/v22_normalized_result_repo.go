package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	run "example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NormalizedMultimodalResultRepo struct {
	db *pgxpool.Pool
}

func NewNormalizedMultimodalResultRepo(db *pgxpool.Pool) *NormalizedMultimodalResultRepo {
	return &NormalizedMultimodalResultRepo{db: db}
}

func (r *NormalizedMultimodalResultRepo) Create(ctx context.Context, in run.CreateNormalizedMultimodalResultInput) (run.NormalizedMultimodalResult, error) {
	const q = `
INSERT INTO normalized_multimodal_results (
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
)
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id,
	created_at_utc,
	updated_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.TaskID,
		in.ResultID,
		string(in.NormalizedKind),
		string(in.NormalizedStatus),
		in.SummaryText,
		nullableFloat64V22(in.ConfidenceScore),
		in.ReasonCode,
		nullableInt64V22(in.ReviewPayloadEvidenceAssetID),
		nullableInt64V22(in.DownstreamPayloadEvidenceAssetID),
	)

	v, err := scanNormalizedMultimodalResult(row)
	if err != nil {
		return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result create: %w", err)
	}
	return v, nil
}

func (r *NormalizedMultimodalResultRepo) FindByID(ctx context.Context, id int64) (run.NormalizedMultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM normalized_multimodal_results
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)
	v, err := scanNormalizedMultimodalResult(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result not found: id=%d", id)
		}
		return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result find by id: %w", err)
	}
	return v, nil
}

func (r *NormalizedMultimodalResultRepo) FindByResultID(ctx context.Context, projectID string, resultID int64) (run.NormalizedMultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM normalized_multimodal_results
WHERE project_id = $1
  AND result_id = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, resultID)
	v, err := scanNormalizedMultimodalResult(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result not found: project_id=%s result_id=%d", projectID, resultID)
		}
		return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result find by result id: %w", err)
	}
	return v, nil
}

func (r *NormalizedMultimodalResultRepo) ListByRunID(ctx context.Context, projectID, runID string) ([]run.NormalizedMultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_id,
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM normalized_multimodal_results
WHERE project_id = $1
  AND run_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("normalized multimodal result list by run id: %w", err)
	}
	defer rows.Close()

	var out []run.NormalizedMultimodalResult
	for rows.Next() {
		v, err := scanNormalizedMultimodalResult(rows)
		if err != nil {
			return nil, fmt.Errorf("normalized multimodal result list by run id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("normalized multimodal result list by run id rows: %w", err)
	}
	return out, nil
}

func (r *NormalizedMultimodalResultRepo) UpdateStatus(ctx context.Context, projectID string, id int64, status run.NormalizedMultimodalResultStatus) (run.NormalizedMultimodalResult, error) {
	const q = `
UPDATE normalized_multimodal_results
SET
	normalized_status = $3,
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
	normalized_kind,
	normalized_status,
	summary_text,
	confidence_score,
	reason_code,
	review_payload_evidence_asset_id,
	downstream_payload_evidence_asset_id,
	created_at_utc,
	updated_at_utc
`
	row := r.db.QueryRow(ctx, q, projectID, id, string(status))
	v, err := scanNormalizedMultimodalResult(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result status update not found: project_id=%s id=%d", projectID, id)
		}
		return run.NormalizedMultimodalResult{}, fmt.Errorf("normalized multimodal result status update: %w", err)
	}
	return v, nil
}

type normalizedMultimodalResultScanner interface {
	Scan(dest ...any) error
}

func scanNormalizedMultimodalResult(s normalizedMultimodalResultScanner) (run.NormalizedMultimodalResult, error) {
	var out run.NormalizedMultimodalResult
	var kind string
	var status string
	var confidence sql.NullFloat64
	var reviewPayload sql.NullInt64
	var downstreamPayload sql.NullInt64

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.RunID,
		&out.TaskID,
		&out.ResultID,
		&kind,
		&status,
		&out.SummaryText,
		&confidence,
		&out.ReasonCode,
		&reviewPayload,
		&downstreamPayload,
		&out.CreatedAtUTC,
		&out.UpdatedAtUTC,
	)
	if err != nil {
		return run.NormalizedMultimodalResult{}, err
	}

	out.NormalizedKind = run.NormalizedMultimodalResultKind(kind)
	out.NormalizedStatus = run.NormalizedMultimodalResultStatus(status)
	out.ConfidenceScore = nullFloat64PtrV22(confidence)
	out.ReviewPayloadEvidenceAssetID = nullInt64PtrV22(reviewPayload)
	out.DownstreamPayloadEvidenceAssetID = nullInt64PtrV22(downstreamPayload)

	return out, nil
}

func nullableFloat64V22(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullFloat64PtrV22(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}
