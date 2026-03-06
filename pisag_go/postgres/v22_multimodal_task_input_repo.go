package postgres

import (
	"context"
	"errors"
	"fmt"

	run "example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MultimodalTaskInputRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalTaskInputRepo(db *pgxpool.Pool) *MultimodalTaskInputRepo {
	return &MultimodalTaskInputRepo{db: db}
}

func (r *MultimodalTaskInputRepo) Create(ctx context.Context, in run.AttachMultimodalTaskInputInput) (run.MultimodalTaskInput, error) {
	const q = `
INSERT INTO multimodal_task_inputs (
	project_id,
	task_id,
	evidence_id,
	input_role,
	seq
) VALUES (
	$1,$2,$3,$4,$5
)
RETURNING
	id,
	project_id,
	task_id,
	evidence_id,
	input_role,
	seq,
	created_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TaskID,
		in.EvidenceID,
		string(in.InputRole),
		in.Seq,
	)

	v, err := scanMultimodalTaskInput(row)
	if err != nil {
		return run.MultimodalTaskInput{}, fmt.Errorf("multimodal task input create: %w", err)
	}
	return v, nil
}

func (r *MultimodalTaskInputRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]run.MultimodalTaskInput, error) {
	const q = `
SELECT
	id,
	project_id,
	task_id,
	evidence_id,
	input_role,
	seq,
	created_at_utc
FROM multimodal_task_inputs
WHERE project_id = $1
  AND task_id = $2
ORDER BY seq ASC, id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("multimodal task input list by task id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalTaskInput
	for rows.Next() {
		v, err := scanMultimodalTaskInput(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal task input list by task id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal task input list by task id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalTaskInputRepo) FindByComposite(ctx context.Context, projectID string, taskID int64, evidenceID int64, inputRole run.MultimodalInputRole, seq int) (run.MultimodalTaskInput, error) {
	const q = `
SELECT
	id,
	project_id,
	task_id,
	evidence_id,
	input_role,
	seq,
	created_at_utc
FROM multimodal_task_inputs
WHERE project_id = $1
  AND task_id = $2
  AND evidence_id = $3
  AND input_role = $4
  AND seq = $5
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, taskID, evidenceID, string(inputRole), seq)

	v, err := scanMultimodalTaskInput(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalTaskInput{}, fmt.Errorf(
				"multimodal task input not found: project_id=%s task_id=%d evidence_id=%d input_role=%s seq=%d",
				projectID, taskID, evidenceID, inputRole, seq,
			)
		}
		return run.MultimodalTaskInput{}, fmt.Errorf("multimodal task input find by composite: %w", err)
	}
	return v, nil
}

type multimodalTaskInputScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalTaskInput(s multimodalTaskInputScanner) (run.MultimodalTaskInput, error) {
	var out run.MultimodalTaskInput
	var inputRole string

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TaskID,
		&out.EvidenceID,
		&inputRole,
		&out.Seq,
		&out.CreatedAtUTC,
	)
	if err != nil {
		return run.MultimodalTaskInput{}, err
	}

	out.InputRole = run.MultimodalInputRole(inputRole)
	return out, nil
}
