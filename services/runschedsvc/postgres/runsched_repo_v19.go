package postgres

import (
	"context"
	"encoding/json"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type DueCronSchedule struct {
	ScheduleID int64
	CronExpr   string
	TimeZone   string
	NextRunUTC time.Time
}

type TickEnqueueRow struct {
	ScheduledRunID int64
	ScheduleID     int64
	ScheduledForUT time.Time
	TraceID        string
	EnqueueKey     string
	Status         string
}

type ClaimedScheduledRun struct {
	ScheduledRunID int64
	ScheduleID     int64
	TraceID        string
}

type CreateRunResult struct {
	ScheduledRunID int64
	RunID          string
	TraceID        string
	Status         string
}

type RunSchedRepoV19 struct {
	db *DB
}

func NewRunSchedRepoV19(db *DB) *RunSchedRepoV19 {
	return &RunSchedRepoV19{db: db}
}

// Used to compute next_map for cron schedules (best-effort; actual claim happens inside tick fn).
func (r *RunSchedRepoV19) ListDueCronSchedules(ctx context.Context, projectID string, nowUTC time.Time, limit int) ([]DueCronSchedule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, cron_expr, timezone, next_run_at_utc
		FROM run_schedules
		WHERE project_id=$1
		  AND enabled=TRUE
		  AND schedule_kind='cron'
		  AND cron_expr IS NOT NULL
		  AND next_run_at_utc <= $2
		ORDER BY priority DESC, next_run_at_utc ASC, id ASC
		LIMIT $3
	`, projectID, nowUTC, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DueCronSchedule, 0, 16)
	for rows.Next() {
		var d DueCronSchedule
		if err := rows.Scan(&d.ScheduleID, &d.CronExpr, &d.TimeZone, &d.NextRunUTC); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *RunSchedRepoV19) TickEnqueue(ctx context.Context, projectID string, nowUTC time.Time, limit int, nextMap map[string]string) ([]TickEnqueueRow, error) {
	b, err := json.Marshal(nextMap)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT scheduled_run_id, schedule_id, scheduled_for_utc, trace_id, enqueue_key, status
		FROM runsched_tick_enqueue_v19($1,$2,$3,$4::jsonb)
	`, projectID, nowUTC, limit, string(b))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TickEnqueueRow
	for rows.Next() {
		var row TickEnqueueRow
		if err := rows.Scan(&row.ScheduledRunID, &row.ScheduleID, &row.ScheduledForUT, &row.TraceID, &row.EnqueueKey, &row.Status); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *RunSchedRepoV19) ClaimQueuedScheduledRuns(ctx context.Context, projectID string, limit int) ([]ClaimedScheduledRun, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT scheduled_run_id, schedule_id, trace_id
		FROM runsched_claim_queued_scheduled_runs_v19($1,$2)
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ClaimedScheduledRun
	for rows.Next() {
		var c ClaimedScheduledRun
		if err := rows.Scan(&c.ScheduledRunID, &c.ScheduleID, &c.TraceID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *RunSchedRepoV19) MarkSkipped(ctx context.Context, projectID string, scheduledRunID int64, status, reasonCode string, reasonEvidenceAssetID int64) error {
	_, err := r.db.Pool.Exec(ctx, `
		SELECT runsched_mark_skipped_v19($1,$2,$3,$4,$5)
	`, projectID, scheduledRunID, status, reasonCode, reasonEvidenceAssetID)
	return err
}

func (r *RunSchedRepoV19) MarkError(ctx context.Context, projectID string, scheduledRunID int64, reasonCode string, errorEvidenceAssetID int64) error {
	_, err := r.db.Pool.Exec(ctx, `
		SELECT runsched_mark_error_v19($1,$2,$3,$4)
	`, projectID, scheduledRunID, reasonCode, errorEvidenceAssetID)
	return err
}

func (r *RunSchedRepoV19) CreateRunForScheduled(ctx context.Context, projectID string, scheduledRunID int64, nowUTC time.Time) (*CreateRunResult, error) {
	var res CreateRunResult
	err := r.db.Pool.QueryRow(ctx, `
		SELECT scheduled_run_id, run_id, trace_id, status
		FROM runsched_create_run_for_scheduled_v19($1,$2,$3)
	`, projectID, scheduledRunID, nowUTC).Scan(&res.ScheduledRunID, &res.RunID, &res.TraceID, &res.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no result for scheduled_run_id=%d", scheduledRunID)
		}
		return nil, err
	}
	return &res, nil
}

// RegisterReasonEvidenceV19 registers a tiny evidence record and returns evidence_assets.id (BIGINT)
// so it can be stored into scheduled_runs.reason_evidence_asset_id.
func (r *RunSchedRepoV19) RegisterReasonEvidenceV19(
	ctx context.Context,
	projectID string,
	traceID string,
	actorType string,
	actorID string,
	reasonCode string,
	message string,
	idempotencyKey string,
) (int64, error) {
	// ---- build deterministic content
	content := fmt.Sprintf("reason_code=%s message=%s", reasonCode, message)
	sum := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(sum[:])

	// ---- evidence_register_v18 -> evidence_ref (uuid)
	var evidenceRef string
	var foundExisting bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT evidence_ref::text, found_existing
		FROM evidence_register_v18(
			$1,  -- project_id
			$2,  -- trace_id (varchar)
			$3,  -- actor_type  (must satisfy created_by_type_ck when materialized)
			$4,  -- actor_id
			$5,  -- media_type  (text/image/audio/video/binary)
			$6,  -- mime_type
			$7,  -- source_kind (MUST be one of evidence_assets_source_kind_ck)
			$8,  -- source_uri
			$9,  -- content_sha256 (len=64)
			$10, -- content_length (>=0)
			$11, -- language
			$12, -- retention_policy (short/standard/legal_hold)
			$13, -- expires_at_utc
			$14  -- idempotency_key
		)
	`, projectID,
		traceID,
		actorType,
		actorID,
		"text",
		"text/plain",
		"generated", // ★ FIX: allowed by evidence_assets_source_kind_ck
		"runschedsvc://reason/"+reasonCode,
		sha,
		int64(len(content)),
		"en",
		"standard",
		time.Now().UTC().Add(24*time.Hour),
		idempotencyKey,
	).Scan(&evidenceRef, &foundExisting)
	if err != nil {
		return 0, err
	}
	_ = foundExisting

	// ---- evidence_assets.id lookup by (project_id, evidence_ref) unique key
	var assetID int64
	err = r.db.Pool.QueryRow(ctx, `
		SELECT id
		FROM evidence_assets
		WHERE project_id = $1
		  AND evidence_ref = $2::uuid
	`, projectID, evidenceRef).Scan(&assetID)
	if err != nil {
		return 0, err
	}
	return assetID, nil
}

func (r *RunSchedRepoV19) BudgetGateCheckV19(ctx context.Context, projectID string, nowUTC time.Time, amount int64) (bool, string, int64, int64, error) {
	var allowed bool
	var reason string
	var remDaily int64
	var remMonthly int64

	err := r.db.Pool.QueryRow(ctx, `
		SELECT allowed, reason_code, remaining_daily, remaining_monthly
		FROM runsched_budget_gate_check_v19($1,$2,$3)
	`, projectID, nowUTC, amount).Scan(&allowed, &reason, &remDaily, &remMonthly)
	if err != nil {
		return false, "budget_gate_db_error", 0, 0, err
	}
	return allowed, reason, remDaily, remMonthly, nil
}

func (r *RunSchedRepoV19) BudgetReserveV19(ctx context.Context, projectID string, scheduledRunID int64, traceID string, amount int64, reasonCode string, reasonAssetID int64) error {
	_, err := r.db.Pool.Exec(ctx, `
		SELECT runsched_budget_reserve_v19($1,$2,$3,$4,$5,$6)
	`, projectID, scheduledRunID, traceID, amount, reasonCode, reasonAssetID)
	return err
}

func (r *RunSchedRepoV19) BudgetConsumeV19(ctx context.Context, projectID string, scheduledRunID int64, runID string, traceID string) error {
	_, err := r.db.Pool.Exec(ctx, `
		SELECT runsched_budget_consume_v19($1,$2,$3::uuid,$4)
	`, projectID, scheduledRunID, runID, traceID)
	return err
}