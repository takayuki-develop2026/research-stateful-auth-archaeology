package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type RunRepository struct{ db *sql.DB }
type RunInputRepository struct{ db *sql.DB }
type RunEventRepository struct{ db *sql.DB }

func NewRunRepository(db *sql.DB) *RunRepository           { return &RunRepository{db: db} }
func NewRunInputRepository(db *sql.DB) *RunInputRepository { return &RunInputRepository{db: db} }
func NewRunEventRepository(db *sql.DB) *RunEventRepository { return &RunEventRepository{db: db} }

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

	var runKey any = nil
	if rr.RunKey != nil && strings.TrimSpace(*rr.RunKey) != "" {
		runKey = strings.TrimSpace(*rr.RunKey)
	}

	// DBのdefault(now())と合わせるため started_at を RETURNING で取る
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO runs(run_id, project_id, trace_id, pipeline_version, status, run_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING started_at
	`, rr.RunID, rr.ProjectID, rr.TraceID, rr.PipelineVersion, string(rr.Status), runKey).Scan(&rr.StartedAt)
	if err != nil {
		return run.Run{}, err
	}
	return rr, nil
}

func (r *RunRepository) CreateOrGetByRunKey(
	ctx context.Context,
	projectID string,
	runKey string,
	newRun func() run.Run,
) (rr run.Run, foundExisting bool, err error) {
	if projectID == "" {
		return run.Run{}, false, errors.New("project_id is required")
	}
	if strings.TrimSpace(runKey) == "" {
		return run.Run{}, false, errors.New("run_key is required")
	}
	runKey = strings.TrimSpace(runKey)

	// NOTE: ak_worker に SELECT 権限が無い前提。
	// この関数は owner/ak 経路でのみ使用すること。
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

	// conflict -> select again
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
	_, err := r.db.ExecContext(ctx, `SELECT public.runs_mark_failed($1::uuid, $2, $3)`, runID, code, msg)
	return err
}

// ✅ Interface(run.RunRepo) を満たすために追加（owner/debug用途）
// NOTE: ak_worker は runs SELECT 権限なしのため、worker経路では使わない前提
func (r *RunRepository) GetTraceID(ctx context.Context, runID string) (string, error) {
	if runID == "" {
		return "", errors.New("run_id is required")
	}
	var traceID string
	err := r.db.QueryRowContext(ctx, `SELECT trace_id FROM runs WHERE run_id=$1`, runID).Scan(&traceID)
	return traceID, err
}

// Insert: enqueue_key は DB トリガーで生成可能なので必須にしない。
// ただし caller が明示したい場合は渡してOK。
func (ri *RunInputRepository) Insert(ctx context.Context, in run.RunInput) error {
	if in.RunID == "" {
		return errors.New("run_id is required")
	}
	if in.TargetURL == "" {
		return errors.New("target_url is required")
	}
	if in.Method == "" {
		in.Method = "GET"
	}
	if len(in.HeadersJSON) == 0 {
		in.HeadersJSON = []byte(`{}`)
	}

	var enqueueKey any = nil
	if strings.TrimSpace(in.EnqueueKey) != "" {
		enqueueKey = strings.TrimSpace(in.EnqueueKey)
	}

	_, err := ri.db.ExecContext(ctx, `
		INSERT INTO run_inputs(run_id, source_id, target_url, method, headers_json, allowlist_key, enqueue_key)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (run_id, enqueue_key) DO NOTHING
	`, in.RunID, in.SourceID, in.TargetURL, in.Method, string(in.HeadersJSON), in.AllowlistKey, enqueueKey)

	return err
}

// ✅ Interface(run.RunInputRepo) を満たすための薄いラッパ
// worker本体は RunInputClaimRepository を使うが、テストが interface 経由で要求してくるので実装する。
// style は最小で CTE 固定でOK。
func (ri *RunInputRepository) ClaimNext(ctx context.Context, workerID string) (*run.ClaimedRunInput, error) {
	cr := NewRunInputClaimRepository(ri.db)
	return cr.ClaimNext(ctx, workerID, ClaimStyleCTE)
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
