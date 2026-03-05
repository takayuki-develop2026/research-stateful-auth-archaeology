package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUtlStatusInvalidArgument  = errors.New("utl_status_invalid_argument")
	ErrUtlStatusPermissionDenied = errors.New("utl_status_permission_denied")
	ErrUtlStatusInvariantFailed  = errors.New("utl_status_invariant_failed")
)

type UtlStatusRepoV62 struct {
	pool *pgxpool.Pool
}

func NewUtlStatusRepoV62(pool *pgxpool.Pool) *UtlStatusRepoV62 {
	return &UtlStatusRepoV62{pool: pool}
}

func (r *UtlStatusRepoV62) MarkProcessed(ctx context.Context, projectID, eventKey, traceID string, runID *string) (string, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(eventKey) == "" || strings.TrimSpace(traceID) == "" {
		return "", fmt.Errorf("%w: project_id/event_key/trace_id required", ErrUtlStatusInvalidArgument)
	}
	var run any = nil
	if runID != nil && strings.TrimSpace(*runID) != "" {
		run = *runID
	}

	const q = `SELECT status FROM public.utl_mark_processed_v6($1::varchar,$2::varchar,$3::uuid,$4::uuid);`

	var st string
	err := r.pool.QueryRow(ctx, q, projectID, eventKey, traceID, run).Scan(&st)
	if err != nil {
		return "", mapUtlStatusPgErr(err)
	}
	return st, nil
}

func (r *UtlStatusRepoV62) MarkNeedsRetry(ctx context.Context, projectID, eventKey, traceID string, runID *string, errorCode *string, errorEvidenceAssetID *int64) (string, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(eventKey) == "" || strings.TrimSpace(traceID) == "" {
		return "", fmt.Errorf("%w: project_id/event_key/trace_id required", ErrUtlStatusInvalidArgument)
	}

	var run any = nil
	if runID != nil && strings.TrimSpace(*runID) != "" {
		run = *runID
	}
	var ec any = nil
	if errorCode != nil && strings.TrimSpace(*errorCode) != "" {
		ec = *errorCode
	}
	var ev any = nil
	if errorEvidenceAssetID != nil {
		ev = *errorEvidenceAssetID
	}

	const q = `SELECT status FROM public.utl_mark_needs_retry_v6($1::varchar,$2::varchar,$3::uuid,$4::uuid,$5::varchar,$6::bigint);`

	var st string
	err := r.pool.QueryRow(ctx, q, projectID, eventKey, traceID, run, ec, ev).Scan(&st)
	if err != nil {
		return "", mapUtlStatusPgErr(err)
	}
	return st, nil
}

func mapUtlStatusPgErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" {
			return fmt.Errorf("%w: %s", ErrUtlStatusPermissionDenied, pgErr.Message)
		}
		msg := pgErr.Message
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
			return fmt.Errorf("%w: %s", ErrUtlStatusInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrUtlStatusInvariantFailed, msg, pgErr.Code)
	}
	return err
}