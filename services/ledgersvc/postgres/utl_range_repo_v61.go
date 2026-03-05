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
	ErrUtlRangeInvalidArgument  = errors.New("utl_range_invalid_argument")
	ErrUtlRangePermissionDenied = errors.New("utl_range_permission_denied")
	ErrUtlRangeInvariantFailed  = errors.New("utl_range_invariant_failed")
)

type UtlRangeRepoV61 struct {
	pool *pgxpool.Pool
}

func NewUtlRangeRepoV61(pool *pgxpool.Pool) *UtlRangeRepoV61 {
	return &UtlRangeRepoV61{pool: pool}
}

type UtlRangeItemV61 struct {
	ID         int64
	EventKey   string
	PostingKey string
	ReceivedAt time.Time
	EventTime  time.Time
	Provider   string
	EventName  string
	ProviderObjectID *string
	TraceID    string
	RunID      *string
	AmountMinor *int64
	Currency   *string
	Status     string
	PayloadEvidenceAssetID *int64
}

func (r *UtlRangeRepoV61) ListRange(ctx context.Context, projectID string, from, to time.Time, status *string, limit int) ([]UtlRangeItemV61, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("%w: project_id required", ErrUtlRangeInvalidArgument)
	}
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, fmt.Errorf("%w: invalid from/to", ErrUtlRangeInvalidArgument)
	}
	if limit <= 0 {
		limit = 500
	}

	const q = `
SELECT
  id,
  event_key,
  posting_key,
  event_time,
  received_at,
  provider,
  event_name,
  provider_object_id,
  trace_id::text,
  CASE WHEN run_id IS NULL THEN NULL ELSE run_id::text END as run_id,
  amount_minor,
  CASE WHEN currency IS NULL THEN NULL ELSE currency::text END as currency,
  status,
  payload_evidence_asset_id
FROM public.utl_list_events_range_v6(
  $1::varchar,
  $2::timestamptz,
  $3::timestamptz,
  $4::varchar,
  $5::int
);`

	var st any = nil
	if status != nil && strings.TrimSpace(*status) != "" {
		st = *status
	}

	rows, err := r.pool.Query(ctx, q, projectID, from, to, st, limit)
	if err != nil {
		return nil, mapUtlRangePgErr(err)
	}
	defer rows.Close()

	out := make([]UtlRangeItemV61, 0, 32)
	for rows.Next() {
		var it UtlRangeItemV61
		err := rows.Scan(
			&it.ID,
			&it.EventKey,
			&it.PostingKey,
			&it.EventTime,
			&it.ReceivedAt,
			&it.Provider,
			&it.EventName,
			&it.ProviderObjectID,
			&it.TraceID,
			&it.RunID,
			&it.AmountMinor,
			&it.Currency,
			&it.Status,
			&it.PayloadEvidenceAssetID,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func mapUtlRangePgErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" {
			return fmt.Errorf("%w: %s", ErrUtlRangePermissionDenied, pgErr.Message)
		}
		msg := pgErr.Message
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
			return fmt.Errorf("%w: %s", ErrUtlRangeInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrUtlRangeInvariantFailed, msg, pgErr.Code)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}