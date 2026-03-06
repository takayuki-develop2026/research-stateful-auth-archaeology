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

type MultimodalResultRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalResultRepo(db *pgxpool.Pool) *MultimodalResultRepo {
	return &MultimodalResultRepo{db: db}
}

func (r *MultimodalResultRepo) Create(ctx context.Context, in run.RegisterMultimodalResultInput) (run.MultimodalResult, error) {
	const q = `
INSERT INTO multimodal_results (
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9
)
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id,
	created_at_utc,
	updated_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.TaskID,
		in.ResultKey,
		string(in.ResultType),
		in.OutputHash,
		in.PayloadEvidenceAssetID,
		nullableInt64V22(in.ConfidenceEvidenceAssetID),
	)

	v, err := scanMultimodalResult(row)
	if err != nil {
		return run.MultimodalResult{}, fmt.Errorf("multimodal result create: %w", err)
	}
	return v, nil
}

func (r *MultimodalResultRepo) FindByProjectAndResultKey(ctx context.Context, projectID, resultKey string) (run.MultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_results
WHERE project_id = $1
  AND result_key = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, resultKey)

	v, err := scanMultimodalResult(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalResult{}, fmt.Errorf("multimodal result not found: project_id=%s result_key=%s", projectID, resultKey)
		}
		return run.MultimodalResult{}, fmt.Errorf("multimodal result find by result key: %w", err)
	}
	return v, nil
}

func (r *MultimodalResultRepo) FindByID(ctx context.Context, id int64) (run.MultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_results
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)

	v, err := scanMultimodalResult(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalResult{}, fmt.Errorf("multimodal result not found: id=%d", id)
		}
		return run.MultimodalResult{}, fmt.Errorf("multimodal result find by id: %w", err)
	}
	return v, nil
}

func (r *MultimodalResultRepo) ListByRunID(ctx context.Context, projectID, runID string) ([]run.MultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_results
WHERE project_id = $1
  AND run_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("multimodal result list by run id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalResult
	for rows.Next() {
		v, err := scanMultimodalResult(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal result list by run id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal result list by run id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalResultRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]run.MultimodalResult, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_id,
	result_key,
	result_type,
	output_hash,
	payload_evidence_asset_id,
	confidence_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_results
WHERE project_id = $1
  AND task_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("multimodal result list by task id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalResult
	for rows.Next() {
		v, err := scanMultimodalResult(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal result list by task id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal result list by task id rows: %w", err)
	}
	return out, nil
}

type multimodalResultScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalResult(s multimodalResultScanner) (run.MultimodalResult, error) {
	var out run.MultimodalResult
	var resultType string
	var confidenceEvidenceAssetID sql.NullInt64

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.RunID,
		&out.TaskID,
		&out.ResultKey,
		&resultType,
		&out.OutputHash,
		&out.PayloadEvidenceAssetID,
		&confidenceEvidenceAssetID,
		&out.CreatedAtUTC,
		&out.UpdatedAtUTC,
	)
	if err != nil {
		return run.MultimodalResult{}, err
	}

	out.ResultType = run.MultimodalResultType(resultType)
	out.ConfidenceEvidenceAssetID = nullInt64PtrV22(confidenceEvidenceAssetID)
	return out, nil
}
