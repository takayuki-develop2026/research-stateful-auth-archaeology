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

type PIIRedactionRepo struct {
	db *pgxpool.Pool
}

func NewPIIRedactionRepo(db *pgxpool.Pool) *PIIRedactionRepo {
	return &PIIRedactionRepo{db: db}
}

func (r *PIIRedactionRepo) Create(ctx context.Context, in run.RegisterPIIRedactionInput) (run.PIIRedaction, error) {
	const q = `
INSERT INTO pii_redactions (
	project_id,
	trace_id,
	evidence_id,
	policy_decision_id,
	rule_key,
	action,
	applied_by_type,
	applied_by_id,
	detail_evidence_asset_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9
)
RETURNING
	id,
	project_id,
	trace_id,
	evidence_id,
	policy_decision_id,
	rule_key,
	action,
	applied_by_type,
	applied_by_id,
	detail_evidence_asset_id,
	created_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.EvidenceID,
		in.PolicyDecisionID,
		in.RuleKey,
		string(in.Action),
		string(in.AppliedByType),
		nullableStringV22(in.AppliedByID),
		in.DetailEvidenceAssetID,
	)

	v, err := scanPIIRedaction(row)
	if err != nil {
		return run.PIIRedaction{}, fmt.Errorf("pii redaction create: %w", err)
	}
	return v, nil
}

func (r *PIIRedactionRepo) ListByEvidenceID(ctx context.Context, projectID string, evidenceID int64) ([]run.PIIRedaction, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	evidence_id,
	policy_decision_id,
	rule_key,
	action,
	applied_by_type,
	applied_by_id,
	detail_evidence_asset_id,
	created_at_utc
FROM pii_redactions
WHERE project_id = $1
  AND evidence_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, evidenceID)
	if err != nil {
		return nil, fmt.Errorf("pii redaction list by evidence id: %w", err)
	}
	defer rows.Close()

	var out []run.PIIRedaction
	for rows.Next() {
		v, err := scanPIIRedaction(rows)
		if err != nil {
			return nil, fmt.Errorf("pii redaction list by evidence id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pii redaction list by evidence id rows: %w", err)
	}
	return out, nil
}

func (r *PIIRedactionRepo) ListByTraceID(ctx context.Context, projectID, traceID string) ([]run.PIIRedaction, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	evidence_id,
	policy_decision_id,
	rule_key,
	action,
	applied_by_type,
	applied_by_id,
	detail_evidence_asset_id,
	created_at_utc
FROM pii_redactions
WHERE project_id = $1
  AND trace_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, traceID)
	if err != nil {
		return nil, fmt.Errorf("pii redaction list by trace id: %w", err)
	}
	defer rows.Close()

	var out []run.PIIRedaction
	for rows.Next() {
		v, err := scanPIIRedaction(rows)
		if err != nil {
			return nil, fmt.Errorf("pii redaction list by trace id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pii redaction list by trace id rows: %w", err)
	}
	return out, nil
}

func (r *PIIRedactionRepo) FindByID(ctx context.Context, id int64) (run.PIIRedaction, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	evidence_id,
	policy_decision_id,
	rule_key,
	action,
	applied_by_type,
	applied_by_id,
	detail_evidence_asset_id,
	created_at_utc
FROM pii_redactions
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)

	v, err := scanPIIRedaction(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.PIIRedaction{}, fmt.Errorf("pii redaction not found: id=%d", id)
		}
		return run.PIIRedaction{}, fmt.Errorf("pii redaction find by id: %w", err)
	}
	return v, nil
}

type piiRedactionScanner interface {
	Scan(dest ...any) error
}

func scanPIIRedaction(s piiRedactionScanner) (run.PIIRedaction, error) {
	var out run.PIIRedaction
	var action string
	var appliedByType string
	var appliedByID sql.NullString

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.EvidenceID,
		&out.PolicyDecisionID,
		&out.RuleKey,
		&action,
		&appliedByType,
		&appliedByID,
		&out.DetailEvidenceAssetID,
		&out.CreatedAtUTC,
	)
	if err != nil {
		return run.PIIRedaction{}, err
	}

	out.Action = run.PIIRedactionAction(action)
	out.AppliedByType = run.PIIRedactionAppliedByType(appliedByType)
	out.AppliedByID = nullStringValueV22(appliedByID)

	return out, nil
}

func nullableStringV22(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullStringValueV22(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
