package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/pisag_go/run"
)

type RunInputClaimRepository struct{ db *sql.DB }

func NewRunInputClaimRepository(db *sql.DB) *RunInputClaimRepository {
	return &RunInputClaimRepository{db: db}
}

type ClaimStyle string

const (
	ClaimStyleCTE             ClaimStyle = "cte_skip_locked"
	ClaimStyleUpdateReturning ClaimStyle = "update_returning"
)

func (r *RunInputClaimRepository) ClaimNextRunInput(
	ctx context.Context,
	workerID string,
	style ClaimStyle,
) (*run.RunInput, error) {
	if workerID == "" {
		return nil, errors.New("worker_id is required")
	}
	if style == "" {
		style = ClaimStyleCTE
	}

	// ✅ DB関数が (0 or 1 row) を返す前提。LIMITは付けない。
	const q = `
SELECT
  id,
  run_id,
  trace_id,
  source_id,
  target_url,
  method,
  headers_json,
  allowlist_key,
  enqueue_key
FROM public.run_inputs_claim_next($1, $2);
`

	var out run.RunInput
	var headersJSON []byte

	err := r.db.QueryRowContext(ctx, q, workerID, string(style)).Scan(
		&out.ID,
		&out.RunID,
		&out.TraceID,
		&out.SourceID,
		&out.TargetURL,
		&out.Method,
		&headersJSON,
		&out.AllowlistKey,
		&out.EnqueueKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	out.HeadersJSON = headersJSON
	return &out, nil
}

func (r *RunInputClaimRepository) MarkRunInputDone(ctx context.Context, inputID int64, workerID string) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE run_inputs
SET claim_status='done',
    last_error_code=NULL,
    last_error_message=NULL
WHERE id=$1 AND claim_status='claimed' AND claimed_by=$2
`, inputID, workerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("MarkRunInputDone: no rows updated (id=%d worker=%s)", inputID, workerID)
	}
	return nil
}

func (r *RunInputClaimRepository) MarkRunInputRetry(ctx context.Context, id int64, workerID, code, msg string) error {
	if code == "fetch_denied" {
		return r.markTerminal(ctx, id, workerID, code, msg)
	}
	if isTerminalHTTP4xx(code) {
		return r.markTerminal(ctx, id, workerID, code, msg)
	}

	res, err := r.db.ExecContext(ctx, `
UPDATE run_inputs
SET claim_status='pending',
    claimed_at=NULL,
    claimed_by=NULL,
    last_error_code=$3,
    last_error_message=$4,
    next_attempt_at = now() + make_interval(secs => LEAST(attempt_count * 10, 300))
WHERE id=$1 AND claim_status='claimed' AND claimed_by=$2
`, id, workerID, code, msg)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("MarkRunInputRetry: no rows updated (id=%d worker=%s code=%s)", id, workerID, code)
	}
	return nil
}

func (r *RunInputClaimRepository) markTerminal(ctx context.Context, id int64, workerID, code, msg string) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE run_inputs
SET claim_status='done',
    last_error_code=$3,
    last_error_message=$4,
    claimed_at=NULL,
    claimed_by=NULL
WHERE id=$1 AND claim_status='claimed' AND claimed_by=$2
`, id, workerID, code, msg)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("markTerminal: no rows updated (id=%d worker=%s code=%s)", id, workerID, code)
	}
	return nil
}

func (r *RunInputClaimRepository) TouchClaim(ctx context.Context, inputID int64, workerID string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE run_inputs
SET claimed_at = now()
WHERE id=$1 AND claim_status='claimed' AND claimed_by=$2
`, inputID, workerID)
	return err
}

func (r *RunInputClaimRepository) SetNextAttemptAt(ctx context.Context, inputID int64, t time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE run_inputs SET next_attempt_at=$2 WHERE id=$1`, inputID, t)
	return err
}

func isTerminalHTTP4xx(code string) bool {
	switch code {
	case "http_400", "http_401", "http_403", "http_404", "http_410":
		return true
	case "http_408", "http_429":
		return false
	default:
		return false
	}
}