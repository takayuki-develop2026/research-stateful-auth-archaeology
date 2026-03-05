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
	ErrUtlNotFound         = errors.New("utl_not_found")
	ErrUtlInvalidArgument  = errors.New("utl_invalid_argument")
	ErrUtlPermissionDenied = errors.New("utl_permission_denied")
	ErrUtlInvariantFailed  = errors.New("utl_invariant_failed")
)

type UtlRepoV6 struct {
	pool *pgxpool.Pool
}

func NewUtlRepoV6(pool *pgxpool.Pool) *UtlRepoV6 {
	return &UtlRepoV6{pool: pool}
}

// Mirrors utl_get_event_v6 return columns we actually need for v14.1.1.
type UtlEventV6 struct {
	ID              int64
	ProjectID        string
	EventSource      string
	Provider         string
	EventName        string
	EventKey         string
	PostingKey       string
	EventTime        time.Time
	ReceivedAt       time.Time
	TraceID          string // uuid as text
	RunID            *string
	AmountMinor      *int64
	Currency         *string
	ProviderObjectID *string
	Status           string
	PayloadEvidenceAssetID *int64
}

func (r *UtlRepoV6) GetByEventKey(ctx context.Context, projectID, eventKey string) (*UtlEventV6, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(eventKey) == "" {
		return nil, fmt.Errorf("%w: project_id and event_key required", ErrUtlInvalidArgument)
	}

	// SECURITY DEFINER function
	const q = `
SELECT
  id,
  project_id,
  event_source,
  provider,
  event_name,
  event_key,
  posting_key,
  event_time,
  received_at,
  trace_id::text,
  CASE WHEN run_id IS NULL THEN NULL ELSE run_id::text END as run_id,
  amount_minor,
  CASE WHEN currency IS NULL THEN NULL ELSE currency::text END as currency,
  provider_object_id,
  status,
  payload_evidence_asset_id
FROM public.utl_get_event_v6($1::varchar, $2::varchar);
`
	row := r.pool.QueryRow(ctx, q, projectID, eventKey)

	var e UtlEventV6
	err := row.Scan(
		&e.ID,
		&e.ProjectID,
		&e.EventSource,
		&e.Provider,
		&e.EventName,
		&e.EventKey,
		&e.PostingKey,
		&e.EventTime,
		&e.ReceivedAt,
		&e.TraceID,
		&e.RunID,
		&e.AmountMinor,
		&e.Currency,
		&e.ProviderObjectID,
		&e.Status,
		&e.PayloadEvidenceAssetID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUtlNotFound
		}
		return nil, mapUtlPgErr(err)
	}
	return &e, nil
}

func mapUtlPgErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" {
			return fmt.Errorf("%w: %s", ErrUtlPermissionDenied, pgErr.Message)
		}
		msg := pgErr.Message
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
			return fmt.Errorf("%w: %s", ErrUtlInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrUtlInvariantFailed, msg, pgErr.Code)
	}
	return err
}