package postgres

import (
	"context"
	"database/sql"
	"errors"
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

// 下の状態遷移は “ak_worker が UPDATE できない” ので DB関数に寄せる。
// ここでは関数呼びの形だけ固定する（migrationsで関数を追加する）。
func (r *RunInputClaimRepository) MarkDone(ctx context.Context, inputID int64, workerID string) error {
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_mark_done($1, $2)`, inputID, workerID)
	return err
}

func (r *RunInputClaimRepository) MarkRetry(ctx context.Context, inputID int64, workerID, code, msg string) error {
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_mark_retry($1, $2, $3, $4)`, inputID, workerID, code, msg)
	return err
}

func (r *RunInputClaimRepository) TouchClaim(ctx context.Context, inputID int64, workerID string) error {
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_touch_claim($1, $2)`, inputID, workerID)
	return err
}

func (r *RunInputClaimRepository) SetNextAttemptAt(ctx context.Context, inputID int64, t time.Time) error {
	_, err := r.db.ExecContext(ctx, `SELECT public.run_inputs_set_next_attempt_at($1, $2)`, inputID, t)
	return err
}