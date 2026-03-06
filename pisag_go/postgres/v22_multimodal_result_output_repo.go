package postgres

import (
	"context"
	"errors"
	"fmt"

	run "example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MultimodalResultOutputRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalResultOutputRepo(db *pgxpool.Pool) *MultimodalResultOutputRepo {
	return &MultimodalResultOutputRepo{db: db}
}

func (r *MultimodalResultOutputRepo) Create(ctx context.Context, in run.AttachMultimodalResultOutputInput) (run.MultimodalResultOutput, error) {
	const q = `
INSERT INTO multimodal_result_outputs (
	project_id,
	result_id,
	evidence_id,
	output_role,
	seq
) VALUES (
	$1,$2,$3,$4,$5
)
RETURNING
	id,
	project_id,
	result_id,
	evidence_id,
	output_role,
	seq,
	created_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.ResultID,
		in.EvidenceID,
		string(in.OutputRole),
		in.Seq,
	)

	v, err := scanMultimodalResultOutput(row)
	if err != nil {
		return run.MultimodalResultOutput{}, fmt.Errorf("multimodal result output create: %w", err)
	}
	return v, nil
}

func (r *MultimodalResultOutputRepo) ListByResultID(ctx context.Context, projectID string, resultID int64) ([]run.MultimodalResultOutput, error) {
	const q = `
SELECT
	id,
	project_id,
	result_id,
	evidence_id,
	output_role,
	seq,
	created_at_utc
FROM multimodal_result_outputs
WHERE project_id = $1
  AND result_id = $2
ORDER BY seq ASC, id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, resultID)
	if err != nil {
		return nil, fmt.Errorf("multimodal result output list by result id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalResultOutput
	for rows.Next() {
		v, err := scanMultimodalResultOutput(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal result output list by result id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal result output list by result id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalResultOutputRepo) FindByComposite(ctx context.Context, projectID string, resultID int64, evidenceID int64, outputRole run.MultimodalOutputRole, seq int) (run.MultimodalResultOutput, error) {
	const q = `
SELECT
	id,
	project_id,
	result_id,
	evidence_id,
	output_role,
	seq,
	created_at_utc
FROM multimodal_result_outputs
WHERE project_id = $1
  AND result_id = $2
  AND evidence_id = $3
  AND output_role = $4
  AND seq = $5
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, resultID, evidenceID, string(outputRole), seq)

	v, err := scanMultimodalResultOutput(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalResultOutput{}, fmt.Errorf(
				"multimodal result output not found: project_id=%s result_id=%d evidence_id=%d output_role=%s seq=%d",
				projectID, resultID, evidenceID, outputRole, seq,
			)
		}
		return run.MultimodalResultOutput{}, fmt.Errorf("multimodal result output find by composite: %w", err)
	}
	return v, nil
}

type multimodalResultOutputScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalResultOutput(s multimodalResultOutputScanner) (run.MultimodalResultOutput, error) {
	var out run.MultimodalResultOutput
	var outputRole string

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.ResultID,
		&out.EvidenceID,
		&outputRole,
		&out.Seq,
		&out.CreatedAtUTC,
	)
	if err != nil {
		return run.MultimodalResultOutput{}, err
	}

	out.OutputRole = run.MultimodalOutputRole(outputRole)
	return out, nil
}
