package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrIngestInvalidArgument  = errors.New("ingest_invalid_argument")
	ErrIngestPermissionDenied = errors.New("ingest_permission_denied")
	ErrIngestInvariantFailed  = errors.New("ingest_invariant_failed")
	ErrIngestNotFound         = errors.New("ingest_not_found")
)

type IngestRepoV141 struct {
	pool *pgxpool.Pool
}

func NewIngestRepoV141(pool *pgxpool.Pool) *IngestRepoV141 {
	return &IngestRepoV141{pool: pool}
}

type IngestAcceptParams struct {
	ProjectID        string
	Mode             string // "single_event" | "range"
	SourceEventKey   string
	FromTS           *time.Time
	ToTS             *time.Time
	Filter           map[string]any
	IdempotencyKey   string
	RunID            string
	TraceID          string
	PolicyVersionID  string
	EvidenceRefs     []string
}

type IngestAcceptResult struct {
	IngestRunID string // uuid text
	Status      string // accepted_created | accepted_exists
}

type IngestClaimResult struct {
	IngestRunID     string
	Mode            string
	SourceEventKey  *string
	FromTS          *time.Time
	ToTS            *time.Time
	FilterJSON      []byte
	IdempotencyKey  string
	RunID           string
	TraceID         string
	PolicyVersionID string
}



func (r *IngestRepoV141) Accept(ctx context.Context, p IngestAcceptParams) (IngestAcceptResult, error) {
	if p.ProjectID == "" || p.Mode == "" || p.IdempotencyKey == "" || p.RunID == "" || p.TraceID == "" || p.PolicyVersionID == "" {
		return IngestAcceptResult{}, fmt.Errorf("%w: required field missing", ErrIngestInvalidArgument)
	}

	filterJSON, err := marshalJSONBObjectOrEmpty(p.Filter)
	if err != nil {
		return IngestAcceptResult{}, fmt.Errorf("%w: filter marshal: %v", ErrIngestInvalidArgument, err)
	}
	evJSON, err := marshalJSONBArrayOrEmpty(p.EvidenceRefs)
	if err != nil {
		return IngestAcceptResult{}, fmt.Errorf("%w: evidence marshal: %v", ErrIngestInvalidArgument, err)
	}

	// NOTE: mode is enum ledger_ingest_mode_v14
	const q = `
SELECT ingest_run_id::text, status
FROM ledger_ingest_run_accept_v14(
  $1::text,
  $2::ledger_ingest_mode_v14,
  $3::text,
  $4::timestamptz,
  $5::timestamptz,
  $6::jsonb,
  $7::text,
  $8::text,
  $9::text,
  $10::text,
  $11::jsonb
);`

	var out IngestAcceptResult

	var from any = nil
	var to any = nil
	if p.FromTS != nil {
		from = *p.FromTS
	}
	if p.ToTS != nil {
		to = *p.ToTS
	}

	err = r.pool.QueryRow(ctx, q,
		p.ProjectID,
		p.Mode,
		nullableText(p.SourceEventKey),
		from,
		to,
		filterJSON, // []byte => jsonb
		p.IdempotencyKey,
		p.RunID,
		p.TraceID,
		p.PolicyVersionID,
		evJSON, // []byte => jsonb array
	).Scan(&out.IngestRunID, &out.Status)

	if err != nil {
		return IngestAcceptResult{}, mapIngestPgErr(err)
	}
	return out, nil
}

func (r *IngestRepoV141) ClaimNext(ctx context.Context, projectID string) (*IngestClaimResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("%w: project_id is required", ErrIngestInvalidArgument)
	}

	const q = `
SELECT
  ingest_run_id::text,
  mode::text,
  source_event_key,
  from_ts,
  to_ts,
  filter::jsonb,
  idempotency_key,
  run_id,
  trace_id,
  policy_version_id
FROM ledger_ingest_run_claim_next_v14($1::text);`

	row := r.pool.QueryRow(ctx, q, projectID)

	var out IngestClaimResult
	var filterRaw []byte
	err := row.Scan(
		&out.IngestRunID,
		&out.Mode,
		&out.SourceEventKey,
		&out.FromTS,
		&out.ToTS,
		&filterRaw,
		&out.IdempotencyKey,
		&out.RunID,
		&out.TraceID,
		&out.PolicyVersionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // nothing to claim
		}
		return nil, mapIngestPgErr(err)
	}
	out.FilterJSON = filterRaw
	return &out, nil
}

func (r *IngestRepoV141) Touch(ctx context.Context, ingestRunID string) error {
	if ingestRunID == "" {
		return fmt.Errorf("%w: ingest_run_id is required", ErrIngestInvalidArgument)
	}
	const q = `SELECT ledger_ingest_run_touch_v14($1::uuid);`
	_, err := r.pool.Exec(ctx, q, ingestRunID)
	return mapIngestPgErr(err)
}

func (r *IngestRepoV141) MarkSucceeded(ctx context.Context, ingestRunID string, stats map[string]any, appendEvidenceRefs []string) error {
	if ingestRunID == "" {
		return fmt.Errorf("%w: ingest_run_id is required", ErrIngestInvalidArgument)
	}
	statsJSON, err := marshalJSONBObjectOrEmpty(stats)
	if err != nil {
		return fmt.Errorf("%w: stats marshal: %v", ErrIngestInvalidArgument, err)
	}
	evJSON, err := marshalJSONBArrayOrEmpty(appendEvidenceRefs)
	if err != nil {
		return fmt.Errorf("%w: evidence marshal: %v", ErrIngestInvalidArgument, err)
	}

	const q = `SELECT ledger_ingest_run_mark_succeeded_v14($1::uuid, $2::jsonb, $3::jsonb);`
	_, execErr := r.pool.Exec(ctx, q, ingestRunID, statsJSON, evJSON)
	return mapIngestPgErr(execErr)
}

func (r *IngestRepoV141) MarkFailedRecorded(ctx context.Context, ingestRunID string, stats map[string]any, appendEvidenceRefs []string) error {
	if ingestRunID == "" {
		return fmt.Errorf("%w: ingest_run_id is required", ErrIngestInvalidArgument)
	}
	statsJSON, err := marshalJSONBObjectOrEmpty(stats)
	if err != nil {
		return fmt.Errorf("%w: stats marshal: %v", ErrIngestInvalidArgument, err)
	}
	evJSON, err := marshalJSONBArrayOrEmpty(appendEvidenceRefs)
	if err != nil {
		return fmt.Errorf("%w: evidence marshal: %v", ErrIngestInvalidArgument, err)
	}

	const q = `SELECT ledger_ingest_run_mark_failed_recorded_v14($1::uuid, $2::jsonb, $3::jsonb);`
	_, execErr := r.pool.Exec(ctx, q, ingestRunID, statsJSON, evJSON)
	return mapIngestPgErr(execErr)
}

func nullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func mapIngestPgErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIngestNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" {
			return fmt.Errorf("%w: %s", ErrIngestPermissionDenied, pgErr.Message)
		}
		msg := pgErr.Message
		if strings.Contains(msg, "is required") || strings.Contains(msg, "must be") || strings.Contains(msg, "invalid") {
			return fmt.Errorf("%w: %s", ErrIngestInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrIngestInvariantFailed, msg, pgErr.Code)
	}
	return err
}