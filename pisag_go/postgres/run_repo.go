package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type RunRepository struct{ db *sql.DB }
type RunInputRepository struct{ db *sql.DB }
type RunEventRepository struct{ db *sql.DB }

func NewRunRepository(db *sql.DB) *RunRepository             { return &RunRepository{db: db} }
func NewRunInputRepository(db *sql.DB) *RunInputRepository   { return &RunInputRepository{db: db} }
func NewRunEventRepository(db *sql.DB) *RunEventRepository   { return &RunEventRepository{db: db} }

func (r *RunRepository) Create(ctx context.Context, rr run.Run) (run.Run, error) {
	if rr.RunID == "" {
		return run.Run{}, errors.New("run_id is required")
	}
	if rr.ProjectID == "" {
		return run.Run{}, errors.New("project_id is required")
	}
	if rr.TraceID == "" {
		return run.Run{}, errors.New("trace_id is required")
	}
	if rr.PipelineVersion == "" {
		return run.Run{}, errors.New("pipeline_version is required")
	}
	if rr.Status == "" {
		return run.Run{}, errors.New("status is required")
	}

	// ✅ *string をそのまま渡さない（nil or string に落とす）
	var runKey any = nil
	if rr.RunKey != nil && strings.TrimSpace(*rr.RunKey) != "" {
		runKey = strings.TrimSpace(*rr.RunKey)
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO runs(run_id, project_id, trace_id, pipeline_version, status, run_key)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, rr.RunID, rr.ProjectID, rr.TraceID, rr.PipelineVersion, string(rr.Status), runKey)
	if err != nil {
		return run.Run{}, err
	}

	rr.StartedAt = time.Now().UTC()
	return rr, nil
}

// v4.2: stable run reuse
// foundExisting=true なら既存runを再利用（同一目的）
func (r *RunRepository) CreateOrGetByRunKey(
	ctx context.Context,
	projectID string,
	runKey string,
	newRun func() run.Run,
) (rr run.Run, foundExisting bool, err error) {
	if projectID == "" {
		return run.Run{}, false, errors.New("project_id is required")
	}
	if runKey == "" {
		return run.Run{}, false, errors.New("run_key is required")
	}

	// 1) try select existing
	var runID, traceID, pipelineVersion, status string
	err = r.db.QueryRowContext(ctx, `
		SELECT run_id, trace_id, pipeline_version, status
		FROM runs
		WHERE project_id=$1 AND run_key=$2
		LIMIT 1
	`, projectID, runKey).Scan(&runID, &traceID, &pipelineVersion, &status)

	if err == nil {
		rk := runKey
		return run.Run{
			RunID:           runID,
			ProjectID:       projectID,
			TraceID:         traceID,
			PipelineVersion: pipelineVersion,
			Status:          run.Status(status),
			RunKey:          &rk,
		}, true, nil
	}
	if err != sql.ErrNoRows {
		return run.Run{}, false, err
	}

	// 2) create new
	rr = newRun()
	if rr.RunID == "" || rr.TraceID == "" || rr.PipelineVersion == "" {
		return run.Run{}, false, errors.New("newRun must set run_id/trace_id/pipeline_version")
	}
	if rr.ProjectID == "" {
		rr.ProjectID = projectID
	}
	rk := runKey
	rr.RunKey = &rk

	if _, err := r.Create(ctx, rr); err == nil {
		return rr, false, nil
	}

	// 3) conflict (someone created first) => select again
	err2 := r.db.QueryRowContext(ctx, `
		SELECT run_id, trace_id, pipeline_version, status
		FROM runs
		WHERE project_id=$1 AND run_key=$2
		LIMIT 1
	`, projectID, runKey).Scan(&runID, &traceID, &pipelineVersion, &status)
	if err2 == nil {
		rk := runKey
		return run.Run{
			RunID:           runID,
			ProjectID:       projectID,
			TraceID:         traceID,
			PipelineVersion: pipelineVersion,
			Status:          run.Status(status),
			RunKey:          &rk,
		}, true, nil
	}

	return run.Run{}, false, err
}

func (r *RunRepository) MarkDone(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("run_id is required")
	}
	// ✅ write-only: UPDATE禁止なので SECURITY DEFINER 関数経由
	_, err := r.db.ExecContext(ctx, `SELECT public.runs_mark_done($1::uuid)`, runID)
	return err
}

func (r *RunRepository) MarkFailed(ctx context.Context, runID string, code string, msg string) error {
	if runID == "" {
		return errors.New("run_id is required")
	}
	if code == "" {
		return errors.New("error_code is required")
	}
	// ✅ write-only: UPDATE禁止なので SECURITY DEFINER 関数経由
	_, err := r.db.ExecContext(ctx, `SELECT public.runs_mark_failed($1::uuid, $2, $3)`, runID, code, msg)
	return err
}

// NOTE: ak_worker は SELECT 権限が無い前提なので、worker側で trace_id が必要なら
// RunRepo.GetTraceID を呼ばず「ClaimNextRunInput の RETURNING で trace_id を一緒に返す」設計が正道。
// ただし owner/ak 用ツールやデバッグ用途で残すならOK。
func (r *RunRepository) GetTraceID(ctx context.Context, runID string) (string, error) {
	var traceID string
	err := r.db.QueryRowContext(ctx, `SELECT trace_id FROM runs WHERE run_id=$1`, runID).Scan(&traceID)
	return traceID, err
}

func (ri *RunInputRepository) Insert(ctx context.Context, in run.RunInput) error {
	if in.RunID == "" {
		return errors.New("run_id is required")
	}
	if in.TargetURL == "" {
		return errors.New("target_url is required")
	}
	if in.Method == "" {
		return errors.New("method is required")
	}
	if in.EnqueueKey == "" {
		return errors.New("enqueue_key is required")
	}
	if len(in.HeadersJSON) == 0 {
		in.HeadersJSON = []byte(`{}`)
	}

	_, err := ri.db.ExecContext(ctx, `
		INSERT INTO run_inputs(run_id, source_id, target_url, method, headers_json, allowlist_key, enqueue_key)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (run_id, enqueue_key) DO NOTHING
	`, in.RunID, in.SourceID, in.TargetURL, in.Method, string(in.HeadersJSON), in.AllowlistKey, in.EnqueueKey)

	return err
}

func (re *RunEventRepository) Append(ctx context.Context, ev run.RunEvent) error {
	if ev.RunID == "" {
		return errors.New("run_id is required")
	}
	if ev.TraceID == "" {
		return errors.New("trace_id is required")
	}
	if ev.EventName == "" {
		return errors.New("event_name is required")
	}
	if ev.Step == "" {
		return errors.New("step is required")
	}
	if ev.Status == "" {
		return errors.New("status is required")
	}
	if len(ev.DataJSON) == 0 {
		ev.DataJSON = []byte(`{}`)
	}

	_, err := re.db.ExecContext(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_name, step, status, message, data_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, ev.RunID, ev.TraceID, ev.EventName, ev.Step, ev.Status, ev.Message, string(ev.DataJSON))
	return err
}