package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrReadInvalidArgument = errors.New("read_invalid_argument")
	ErrReadNotFound        = errors.New("read_not_found")
)

type LedgerIngestRunReadRepo struct {
	pool *pgxpool.Pool
}

func NewLedgerIngestRunReadRepo(pool *pgxpool.Pool) *LedgerIngestRunReadRepo {
	return &LedgerIngestRunReadRepo{pool: pool}
}

type LedgerIngestRunRow struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"project_id"`
	Mode           string          `json:"mode"`
	SourceEventKey *string         `json:"source_event_key,omitempty"`
	FromTS         *time.Time      `json:"from_ts,omitempty"`
	ToTS           *time.Time      `json:"to_ts,omitempty"`
	Filter         json.RawMessage `json:"filter"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	RunID          string          `json:"run_id"`
	TraceID        string          `json:"trace_id"`
	PolicyVersionID string         `json:"policy_version_id"`
	Stats          json.RawMessage `json:"stats"`
	EvidenceRefs   []string        `json:"evidence_refs"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (r *LedgerIngestRunReadRepo) List(
	ctx context.Context,
	projectID string,
	status *string,
	from *time.Time,
	to *time.Time,
	limit int,
) ([]LedgerIngestRunRow, error) {

	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("%w: project_id required", ErrReadInvalidArgument)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Default window: last 7 days if not provided
	var fromTS time.Time
	var toTS time.Time
	if from == nil {
		fromTS = time.Now().Add(-7 * 24 * time.Hour).UTC()
	} else {
		fromTS = from.UTC()
	}
	if to == nil {
		toTS = time.Now().Add(1 * time.Minute).UTC()
	} else {
		toTS = to.UTC()
	}

	var st any = nil
	if status != nil && strings.TrimSpace(*status) != "" {
		st = *status
	}

	const q = `
SELECT
  id::text,
  project_id,
  mode::text,
  source_event_key,
  from_ts,
  to_ts,
  filter::jsonb,
  idempotency_key,
  status::text,
  run_id,
  trace_id,
  policy_version_id,
  stats::jsonb,
  evidence_refs::jsonb,
  created_at,
  updated_at
FROM public.ledger_ingest_runs
WHERE project_id = $1::text
  AND created_at >= $2::timestamptz
  AND created_at <  $3::timestamptz
  AND ($4::text IS NULL OR status = $4::ledger_ingest_status_v14)
ORDER BY created_at DESC
LIMIT $5::int;
`
	rows, err := r.pool.Query(ctx, q, projectID, fromTS, toTS, st, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LedgerIngestRunRow, 0, 32)
	for rows.Next() {
		var row LedgerIngestRunRow
		var filterRaw []byte
		var statsRaw []byte
		var evRefsRaw []byte
		err := rows.Scan(
			&row.ID,
			&row.ProjectID,
			&row.Mode,
			&row.SourceEventKey,
			&row.FromTS,
			&row.ToTS,
			&filterRaw,
			&row.IdempotencyKey,
			&row.Status,
			&row.RunID,
			&row.TraceID,
			&row.PolicyVersionID,
			&statsRaw,
			&evRefsRaw,
			&row.CreatedAt,
			&row.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		row.Filter = json.RawMessage(filterRaw)
		row.Stats = json.RawMessage(statsRaw)

		// evidence_refs stored as jsonb array of uuid strings
		var refs []string
		if len(evRefsRaw) > 0 {
			_ = json.Unmarshal(evRefsRaw, &refs)
		}
		row.EvidenceRefs = refs

		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r *LedgerIngestRunReadRepo) Get(ctx context.Context, projectID, ingestRunID string) (*LedgerIngestRunRow, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(ingestRunID) == "" {
		return nil, fmt.Errorf("%w: project_id/ingest_run_id required", ErrReadInvalidArgument)
	}

	const q = `
SELECT
  id::text,
  project_id,
  mode::text,
  source_event_key,
  from_ts,
  to_ts,
  filter::jsonb,
  idempotency_key,
  status::text,
  run_id,
  trace_id,
  policy_version_id,
  stats::jsonb,
  evidence_refs::jsonb,
  created_at,
  updated_at
FROM public.ledger_ingest_runs
WHERE project_id = $1::text AND id = $2::uuid
LIMIT 1;
`
	var row LedgerIngestRunRow
	var filterRaw []byte
	var statsRaw []byte
	var evRefsRaw []byte

	err := r.pool.QueryRow(ctx, q, projectID, ingestRunID).Scan(
		&row.ID,
		&row.ProjectID,
		&row.Mode,
		&row.SourceEventKey,
		&row.FromTS,
		&row.ToTS,
		&filterRaw,
		&row.IdempotencyKey,
		&row.Status,
		&row.RunID,
		&row.TraceID,
		&row.PolicyVersionID,
		&statsRaw,
		&evRefsRaw,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReadNotFound
		}
		return nil, err
	}

	row.Filter = json.RawMessage(filterRaw)
	row.Stats = json.RawMessage(statsRaw)

	var refs []string
	if len(evRefsRaw) > 0 {
		_ = json.Unmarshal(evRefsRaw, &refs)
	}
	row.EvidenceRefs = refs

	return &row, nil
}