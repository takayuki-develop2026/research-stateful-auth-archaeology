package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/pisag_go/run"
)

type RunRepository struct{ db *sql.DB }
type RunInputRepository struct{ db *sql.DB }
type RunEventRepository struct{ db *sql.DB }

func NewRunRepository(db *sql.DB) *RunRepository         { return &RunRepository{db: db} }
func NewRunInputRepository(db *sql.DB) *RunInputRepository { return &RunInputRepository{db: db} }
func NewRunEventRepository(db *sql.DB) *RunEventRepository { return &RunEventRepository{db: db} }

func (r *RunRepository) Create(ctx context.Context, rr run.Run) (run.Run, error) {
	// Repoは「主語を作らない」。UseCase側で run_id/trace_id を確定して渡す前提。
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

	// SELECT権限が無い前提なので RETURNING は使わず Exec のみ
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO runs(run_id, project_id, trace_id, pipeline_version, status)
		VALUES ($1, $2, $3, $4, $5)
	`, rr.RunID, rr.ProjectID, rr.TraceID, rr.PipelineVersion, string(rr.Status))
	if err != nil {
		return run.Run{}, err
	}

	// DB now() とは厳密一致しないが、呼び出し側のログ用途にUTCを入れて返す（不要ならゼロでもOK）
	rr.StartedAt = time.Now().UTC()
	return rr, nil
}

func (r *RunRepository) MarkDone(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("run_id is required")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE runs
		SET status=$2, finished_at=now()
		WHERE run_id=$1
	`, runID, string(run.StatusDone))
	return err
}

func (r *RunRepository) MarkFailed(ctx context.Context, runID string, code string, msg string) error {
	if runID == "" {
		return errors.New("run_id is required")
	}
	if code == "" {
		return errors.New("error_code is required")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE runs
		SET status=$2, finished_at=now(), error_code=$3, error_message=$4
		WHERE run_id=$1
	`, runID, string(run.StatusFailed), code, msg)
	return err
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
	if len(in.HeadersJSON) == 0 {
		in.HeadersJSON = []byte(`{}`)
	}

	_, err := ri.db.ExecContext(ctx, `
		INSERT INTO run_inputs(run_id, source_id, target_url, method, headers_json, allowlist_key)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
	`, in.RunID, in.SourceID, in.TargetURL, in.Method, string(in.HeadersJSON), in.AllowlistKey)
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