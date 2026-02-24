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

// ClaimNext claims next pending run_input and returns (0 or 1 row).
// DB function does the locking + returning trace_id, so worker SELECT is unnecessary.
func (r *RunInputClaimRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	style ClaimStyle,
) (*run.ClaimedRunInput, error) {
	if workerID == "" {
		return nil, errors.New("worker_id is required")
	}
	if style == "" {
		style = ClaimStyleCTE
	}

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

	var out run.ClaimedRunInput
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

// MarkDone: EXECUTE ONLY (no direct UPDATE).
func (r *RunInputClaimRepository) MarkDone(ctx context.Context, inputID int64, workerID string) error {
	if inputID <= 0 {
		return errors.New("input_id is required")
	}
	if workerID == "" {
		return errors.New("worker_id is required")
	}

	// Function:
	// - verify (id, claimed_by=workerID, claim_status='claimed')
	// - set claim_status='done' and clear last_error fields
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_mark_done($1::bigint, $2)`, inputID, workerID)
	return err
}

// MarkRetry: EXECUTE ONLY (no direct UPDATE).
func (r *RunInputClaimRepository) MarkRetry(ctx context.Context, inputID int64, workerID, code, msg string) error {
	if inputID <= 0 {
		return errors.New("input_id is required")
	}
	if workerID == "" {
		return errors.New("worker_id is required")
	}
	if code == "" {
		return errors.New("error_code is required")
	}

	// Function:
	// - terminal rule inside DB: fetch_denied, http_400/401/403/404/410 => done (terminal)
	// - retryable => pending with backoff and error fields
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_mark_retry($1::bigint, $2, $3, $4)`,
		inputID, workerID, code, msg,
	)
	return err
}

// TouchClaim: EXECUTE ONLY (no direct UPDATE).
func (r *RunInputClaimRepository) TouchClaim(ctx context.Context, inputID int64, workerID string) error {
	if inputID <= 0 {
		return errors.New("input_id is required")
	}
	if workerID == "" {
		return errors.New("worker_id is required")
	}
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_touch_claim($1::bigint, $2)`, inputID, workerID)
	return err
}

// SetNextAttemptAt: EXECUTE ONLY.
// Note: worker運用でこれを使うなら関数化が必須。
func (r *RunInputClaimRepository) SetNextAttemptAt(ctx context.Context, inputID int64, t time.Time) error {
	if inputID <= 0 {
		return errors.New("input_id is required")
	}
	_, err := r.db.ExecContext(
		ctx,
		`SELECT public.run_inputs_set_next_attempt_at($1::bigint, $2::timestamptz)`,
		inputID,
		t.UTC(),
	)
	return err
}

/* ---------- helpers (optional) ---------- */

// Debug helper (owner用). Workerは使わない前提。
func (r *RunInputClaimRepository) DebugCountPending(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM public.run_inputs WHERE claim_status='pending'`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *RunInputClaimRepository) EnsureFunctionsExist(ctx context.Context) error {
	// Minimal sanity check: call a pure function if exists (optional).
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_is_terminal_code('http_404')`)
	if err != nil {
		return fmt.Errorf("missing required function(s): %w", err)
	}
	return nil
}