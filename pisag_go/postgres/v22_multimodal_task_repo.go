package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	run "example.com/pisag_go/run"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MultimodalTaskRepo struct {
	db *pgxpool.Pool
}

func NewMultimodalTaskRepo(db *pgxpool.Pool) *MultimodalTaskRepo {
	return &MultimodalTaskRepo{db: db}
}

func (r *MultimodalTaskRepo) Create(ctx context.Context, in run.RegisterMultimodalTaskInput) (run.MultimodalTask, error) {
	const q = `
INSERT INTO multimodal_tasks (
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	soft_error_evidence_asset_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
)
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	started_at_utc,
	finished_at_utc,
	soft_error_evidence_asset_id,
	created_at_utc,
	updated_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.TaskKey,
		string(in.TaskType),
		in.PipelineVersion,
		in.PolicyVersionStr,
		in.InputHash,
		string(in.Status),
		in.RouterPlanEvidenceAssetID,
		in.OptionsEvidenceAssetID,
		nullableInt64V22(in.ModelRunID),
		nullableInt64V22(in.SoftErrorEvidenceAssetID),
	)

	task, err := scanMultimodalTask(row)
	if err != nil {
		return run.MultimodalTask{}, fmt.Errorf("multimodal task create: %w", err)
	}
	return task, nil
}

func (r *MultimodalTaskRepo) FindByProjectAndTaskKey(ctx context.Context, projectID, taskKey string) (run.MultimodalTask, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	started_at_utc,
	finished_at_utc,
	soft_error_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_tasks
WHERE project_id = $1
  AND task_key = $2
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, projectID, taskKey)

	task, err := scanMultimodalTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalTask{}, fmt.Errorf("multimodal task not found: project_id=%s task_key=%s", projectID, taskKey)
		}
		return run.MultimodalTask{}, fmt.Errorf("multimodal task find by task key: %w", err)
	}
	return task, nil
}

func (r *MultimodalTaskRepo) FindByID(ctx context.Context, id int64) (run.MultimodalTask, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	started_at_utc,
	finished_at_utc,
	soft_error_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_tasks
WHERE id = $1
LIMIT 1
`
	row := r.db.QueryRow(ctx, q, id)

	task, err := scanMultimodalTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalTask{}, fmt.Errorf("multimodal task not found: id=%d", id)
		}
		return run.MultimodalTask{}, fmt.Errorf("multimodal task find by id: %w", err)
	}
	return task, nil
}

func (r *MultimodalTaskRepo) ListByRunID(ctx context.Context, projectID, runID string) ([]run.MultimodalTask, error) {
	const q = `
SELECT
	id,
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	started_at_utc,
	finished_at_utc,
	soft_error_evidence_asset_id,
	created_at_utc,
	updated_at_utc
FROM multimodal_tasks
WHERE project_id = $1
  AND run_id = $2
ORDER BY id ASC
`
	rows, err := r.db.Query(ctx, q, projectID, runID)
	if err != nil {
		return nil, fmt.Errorf("multimodal task list by run id: %w", err)
	}
	defer rows.Close()

	var out []run.MultimodalTask
	for rows.Next() {
		v, err := scanMultimodalTask(rows)
		if err != nil {
			return nil, fmt.Errorf("multimodal task list by run id scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("multimodal task list by run id rows: %w", err)
	}
	return out, nil
}

func (r *MultimodalTaskRepo) MarkRunning(ctx context.Context, projectID string, taskID int64, startedAtUTC time.Time) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:    projectID,
		TaskID:       taskID,
		Status:       run.MultimodalTaskStatusRunning,
		StartedAtUTC: &startedAtUTC,
	})
}

func (r *MultimodalTaskRepo) MarkSucceeded(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:     projectID,
		TaskID:        taskID,
		Status:        run.MultimodalTaskStatusSucceeded,
		FinishedAtUTC: &finishedAtUTC,
	})
}

func (r *MultimodalTaskRepo) MarkReviewRequired(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:                projectID,
		TaskID:                   taskID,
		Status:                   run.MultimodalTaskStatusReviewRequired,
		FinishedAtUTC:            &finishedAtUTC,
		SoftErrorEvidenceAssetID: softErrorEvidenceAssetID,
	})
}

func (r *MultimodalTaskRepo) MarkSkippedBudget(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:                projectID,
		TaskID:                   taskID,
		Status:                   run.MultimodalTaskStatusSkippedBudget,
		FinishedAtUTC:            &finishedAtUTC,
		SoftErrorEvidenceAssetID: softErrorEvidenceAssetID,
	})
}

func (r *MultimodalTaskRepo) MarkFailedSoft(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:                projectID,
		TaskID:                   taskID,
		Status:                   run.MultimodalTaskStatusFailedSoft,
		FinishedAtUTC:            &finishedAtUTC,
		SoftErrorEvidenceAssetID: softErrorEvidenceAssetID,
	})
}

func (r *MultimodalTaskRepo) MarkBlockedPolicy(ctx context.Context, projectID string, taskID int64, finishedAtUTC time.Time, softErrorEvidenceAssetID *int64) (run.MultimodalTask, error) {
	return r.updateStatus(ctx, updateMultimodalTaskStatusInput{
		ProjectID:                projectID,
		TaskID:                   taskID,
		Status:                   run.MultimodalTaskStatusBlockedPolicy,
		FinishedAtUTC:            &finishedAtUTC,
		SoftErrorEvidenceAssetID: softErrorEvidenceAssetID,
	})
}

type updateMultimodalTaskStatusInput struct {
	ProjectID                string
	TaskID                   int64
	Status                   run.MultimodalTaskStatus
	StartedAtUTC             *time.Time
	FinishedAtUTC            *time.Time
	SoftErrorEvidenceAssetID *int64
}

func (r *MultimodalTaskRepo) updateStatus(ctx context.Context, in updateMultimodalTaskStatusInput) (run.MultimodalTask, error) {
	const q = `
UPDATE multimodal_tasks
SET
	status = $3,
	started_at_utc = COALESCE($4::timestamptz, started_at_utc),
	finished_at_utc = COALESCE($5::timestamptz, finished_at_utc),
	soft_error_evidence_asset_id = COALESCE($6::bigint, soft_error_evidence_asset_id),
	updated_at_utc = now()
WHERE project_id = $1
  AND id = $2
RETURNING
	id,
	project_id,
	trace_id,
	run_id,
	task_key,
	task_type,
	pipeline_version,
	policy_version_str,
	input_hash,
	status,
	router_plan_evidence_asset_id,
	options_evidence_asset_id,
	model_run_id,
	started_at_utc,
	finished_at_utc,
	soft_error_evidence_asset_id,
	created_at_utc,
	updated_at_utc
`
	row := r.db.QueryRow(ctx, q,
		in.ProjectID,
		in.TaskID,
		string(in.Status),
		nullableTimeV22(in.StartedAtUTC),
		nullableTimeV22(in.FinishedAtUTC),
		nullableInt64V22(in.SoftErrorEvidenceAssetID),
	)

	task, err := scanMultimodalTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalTask{}, fmt.Errorf("multimodal task status update not found: project_id=%s task_id=%d", in.ProjectID, in.TaskID)
		}
		return run.MultimodalTask{}, fmt.Errorf("multimodal task status update: %w", err)
	}
	return task, nil
}

type multimodalTaskScanner interface {
	Scan(dest ...any) error
}

func scanMultimodalTask(s multimodalTaskScanner) (run.MultimodalTask, error) {
	var out run.MultimodalTask

	var taskType string
	var status string
	var modelRunID sql.NullInt64
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	var softErrorEvidenceAssetID sql.NullInt64

	err := s.Scan(
		&out.ID,
		&out.ProjectID,
		&out.TraceID,
		&out.RunID,
		&out.TaskKey,
		&taskType,
		&out.PipelineVersion,
		&out.PolicyVersionStr,
		&out.InputHash,
		&status,
		&out.RouterPlanEvidenceAssetID,
		&out.OptionsEvidenceAssetID,
		&modelRunID,
		&startedAt,
		&finishedAt,
		&softErrorEvidenceAssetID,
		&out.CreatedAtUTC,
		&out.UpdatedAtUTC,
	)
	if err != nil {
		return run.MultimodalTask{}, err
	}

	out.TaskType = run.MultimodalTaskType(taskType)
	out.Status = run.MultimodalTaskStatus(status)
	out.ModelRunID = nullInt64PtrV22(modelRunID)
	out.StartedAtUTC = nullTimePtrV22(startedAt)
	out.FinishedAtUTC = nullTimePtrV22(finishedAt)
	out.SoftErrorEvidenceAssetID = nullInt64PtrV22(softErrorEvidenceAssetID)

	return out, nil
}

func nullableInt64V22(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTimeV22(v *time.Time) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt64PtrV22(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func nullTimePtrV22(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func (r *MultimodalTaskRepo) ClaimNextQueuedOCRTask(ctx context.Context, projectID string) (run.MultimodalTask, bool, error) {
	const q = `
WITH picked AS (
	SELECT id
	FROM multimodal_tasks
	WHERE project_id = $1
	  AND task_type = 'ocr'
	  AND status = 'queued'
	ORDER BY id ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
UPDATE multimodal_tasks t
SET
	updated_at_utc = now()
FROM picked
WHERE t.id = picked.id
RETURNING
	t.id,
	t.project_id,
	t.trace_id,
	t.run_id,
	t.task_key,
	t.task_type,
	t.pipeline_version,
	t.policy_version_str,
	t.input_hash,
	t.status,
	t.router_plan_evidence_asset_id,
	t.options_evidence_asset_id,
	t.model_run_id,
	t.started_at_utc,
	t.finished_at_utc,
	t.soft_error_evidence_asset_id,
	t.created_at_utc,
	t.updated_at_utc
`
	row := r.db.QueryRow(ctx, q, projectID)

	task, err := scanMultimodalTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return run.MultimodalTask{}, false, nil
		}
		return run.MultimodalTask{}, false, fmt.Errorf("claim next queued ocr task: %w", err)
	}
	return task, true, nil
}