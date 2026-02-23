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
	switch style {
	case ClaimStyleCTE:
		return r.claimCTE(ctx, workerID)
	case ClaimStyleUpdateReturning:
		return r.claimUpdateReturning(ctx, workerID)
	default:
		return nil, errors.New("unknown claim style")
	}
}

func (r *RunInputClaimRepository) claimCTE(ctx context.Context, workerID string) (*run.RunInput, error) {
	const q = `
WITH cte AS (
  SELECT id
  FROM run_inputs
  WHERE claim_status='pending'
    AND next_attempt_at <= now()
  ORDER BY created_at ASC, id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE run_inputs ri
SET claim_status='claimed',
    claimed_at=now(),
    claimed_by=$1,
    attempt_count=attempt_count+1
FROM cte
WHERE ri.id = cte.id
RETURNING ri.id, ri.run_id, ri.source_id, ri.target_url, ri.method, ri.headers_json, ri.allowlist_key;
`
	var out run.RunInput
	var headersJSON []byte

	err := r.db.QueryRowContext(ctx, q, workerID).Scan(
		&out.ID,
		&out.RunID,
		&out.SourceID,
		&out.TargetURL,
		&out.Method,
		&headersJSON,
		&out.AllowlistKey,
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

func (r *RunInputClaimRepository) claimUpdateReturning(ctx context.Context, workerID string) (*run.RunInput, error) {
	const q = `
UPDATE run_inputs ri
SET claim_status='claimed',
    claimed_at=now(),
    claimed_by=$1,
    attempt_count=attempt_count+1
WHERE ri.id = (
  SELECT id
  FROM run_inputs
  WHERE claim_status='pending'
    AND next_attempt_at <= now()
  ORDER BY created_at ASC, id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING ri.id, ri.run_id, ri.source_id, ri.target_url, ri.method, ri.headers_json, ri.allowlist_key;
`
	var out run.RunInput
	var headersJSON []byte

	err := r.db.QueryRowContext(ctx, q, workerID).Scan(
		&out.ID,
		&out.RunID,
		&out.SourceID,
		&out.TargetURL,
		&out.Method,
		&headersJSON,
		&out.AllowlistKey,
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
	// ----------------------------
	// 1) 終端化ルール（重要）
	// ----------------------------
	if code == "fetch_denied" {
		return r.markTerminal(ctx, id, workerID, code, msg)
	}

	// 4xx は基本 terminal（ただし 408/429 は retry 寄り）
	if isTerminalHTTP4xx(code) {
		return r.markTerminal(ctx, id, workerID, code, msg)
	}

	// ----------------------------
	// 2) retry（backoff）
	// ----------------------------
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
	// worker 側で "http_404" みたいに入れる想定
	switch code {
	case "http_400", "http_401", "http_403", "http_404", "http_410":
		return true
	// retry寄り：
	case "http_408", "http_429":
		return false
	default:
		return false
	}
}