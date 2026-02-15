package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

type RunRow struct {
	RunID   string
	TraceID string
}

func (s *Store) PickQueuedRun(ctx context.Context, workerID string) (RunRow, bool, error) {
	_ = workerID // future: picked_by

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RunRow{}, false, err
	}
	defer tx.Rollback(ctx)

	var r RunRow
	err = tx.QueryRow(ctx, `
		SELECT run_id, trace_id
		FROM runs
		WHERE state = 'queued'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&r.RunID, &r.TraceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RunRow{}, false, nil
		}
		return RunRow{}, false, err
	}

	r.RunID = NormalizeRunID(r.RunID)
	r.TraceID = strings.TrimSpace(r.TraceID)

	if !IsValidRunID(r.RunID) {
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET state='review_required', status='review_required', result=COALESCE(NULLIF(result,''),'pending'), updated_at=now()
			WHERE run_id=$1
		`, r.RunID)
		_ = tx.Commit(ctx)
		return RunRow{}, false, fmt.Errorf("invalid run_id length: %q", r.RunID)
	}

	if r.TraceID == "" {
		r.TraceID = NewTraceID()
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id=$2
			WHERE run_id=$1
		`, r.RunID, r.TraceID)
		if err != nil {
			return RunRow{}, false, err
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE runs
		SET state='running', status=NULL, result=COALESCE(NULLIF(result,''),'pending'), updated_at=now()
		WHERE run_id=$1 AND state='queued'
	`, r.RunID)
	if err != nil {
		return RunRow{}, false, err
	}
	if tag.RowsAffected() != 1 {
		return RunRow{}, false, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return RunRow{}, false, err
	}
	return r, true, nil
}

func (s *Store) GetRunProjectID(ctx context.Context, runID string) (string, error) {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var projectID string
	err := s.db.QueryRow(ctx, `
		SELECT project_id
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(&projectID)
	return strings.TrimSpace(projectID), err
}

func (s *Store) GetRunState(ctx context.Context, runID string) (string, error) {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var st string
	err := s.db.QueryRow(ctx, `
		SELECT state
		FROM runs
		WHERE run_id=$1
	`, runID).Scan(&st)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(st), nil
}

func (s *Store) GetRunModeFromEnqueuedEvent(ctx context.Context, runID string) (int, error) {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var modeText string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(payload->>'mode', '')
		FROM run_events
		WHERE run_id = $1 AND event_name = 'run.enqueued'
		ORDER BY event_seq ASC
		LIMIT 1
	`, runID).Scan(&modeText)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	modeText = strings.TrimSpace(modeText)
	if modeText == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(modeText)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

func (s *Store) AppendEvent(ctx context.Context, runID, traceID, eventName string, payload map[string]any) error {
	return s.appendEventInternal(ctx, runID, traceID, eventName, payload, "", false)
}

func (s *Store) AppendEventAndStatus(ctx context.Context, runID, traceID, eventName, newState string, payload map[string]any) error {
	return s.appendEventInternal(ctx, runID, traceID, eventName, payload, newState, true)
}

func (s *Store) appendEventInternal(
	ctx context.Context,
	runID, traceID, eventName string,
	payload map[string]any,
	newState string,
	updateState bool,
) error {
	runID = NormalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	seq, err := nextEventSeqTx(ctx, tx, runID)
	if err != nil {
		return err
	}

	if traceID != "" {
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id = CASE WHEN COALESCE(NULLIF(BTRIM(trace_id),''),'') = '' THEN $2 ELSE trace_id END
			WHERE run_id=$1
		`, runID, traceID)
	}

	pb := MarshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, seq, eventName, pb)
	if err != nil {
		return err
	}

	if !updateState {
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET next_event_seq=$2, updated_at=now()
			WHERE run_id=$1
		`, runID, seq+1)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// state/status/result policy
	var status any = nil
	var result any = nil

	switch newState {
	case "done":
		status = nil
		result = "success"
	case "failed":
		status = "failed"
		result = "failed"
	case "review_required":
		status = "review_required"
		result = "pending"
	default:
		status = nil
		result = nil
	}

	var tag pgconn.CommandTag
	if result == nil {
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET state=$2,
			    status=$3,
			    next_event_seq=$4,
			    updated_at=now()
			WHERE run_id=$1
		`, runID, newState, status, seq+1)
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET state=$2,
			    status=$3,
			    result=$4,
			    next_event_seq=$5,
			    updated_at=now()
			WHERE run_id=$1
		`, runID, newState, status, result, seq+1)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("runs update affected=%d (run_id=%s)", tag.RowsAffected(), runID)
	}

	return tx.Commit(ctx)
}

func nextEventSeqTx(ctx context.Context, tx pgx.Tx, runID string) (int64, error) {
	runID = NormalizeRunID(runID)

	var next int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(next_event_seq,0), 1)
		FROM runs
		WHERE run_id=$1
		FOR UPDATE
	`, runID).Scan(&next)
	if err != nil {
		return 0, err
	}

	var maxSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0)
		FROM run_events
		WHERE run_id=$1
	`, runID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}

	safe := next
	if maxSeq+1 > safe {
		safe = maxSeq + 1
	}

	if safe != next {
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET next_event_seq = $2,
			    updated_at = now()
			WHERE run_id = $1
		`, runID, safe)
		if err != nil {
			return 0, err
		}
	}

	return safe, nil
}

func (s *Store) UpsertRunArtifact(ctx context.Context, runID, traceID, kind string, content any) error {
	runID = NormalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	kind = strings.TrimSpace(kind)

	if traceID == "" {
		return fmt.Errorf("trace_id is required for run_artifacts (run_id=%s kind=%s)", runID, kind)
	}
	if kind == "" {
		return fmt.Errorf("artifact_kind is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	b, err := json.Marshal(content)
	if err != nil {
		b = []byte(`{}`)
	}

	runIDText := runID // bpchar vs text parameter split

	_, err = s.db.Exec(ctx, `
		INSERT INTO run_artifacts(
			run_id,
			artifact_kind,
			content_json,
			created_at,
			updated_at,
			schema_version,
			trace_id,
			artifact_ref_kind,
			artifact_ref_run_id,
			artifact_ref_trace_id,
			trace_trace_id
		)
		VALUES (
			$1,
			$2,
			$3::jsonb,
			now(),
			now(),
			$4,
			$5,
			$2,
			$6,
			$5,
			$5
		)
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET
			content_json = EXCLUDED.content_json,
			updated_at  = now(),
			schema_version        = EXCLUDED.schema_version,
			trace_id              = EXCLUDED.trace_id,
			artifact_ref_kind     = EXCLUDED.artifact_ref_kind,
			artifact_ref_run_id   = EXCLUDED.artifact_ref_run_id,
			artifact_ref_trace_id = EXCLUDED.artifact_ref_trace_id,
			trace_trace_id        = EXCLUDED.trace_trace_id
	`, runID, kind, string(b), RunArtifactSchemaVersion, traceID, runIDText)

	return err
}

func (s *Store) BeginAttemptTx(ctx context.Context, runID, traceID, projectID, workerID string) (int, error) {
	runID = NormalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	if traceID == "" {
		return 0, fmt.Errorf("trace_id is required for BeginAttemptTx")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	seq, err := nextEventSeqTx(ctx, tx, runID)
	if err != nil {
		return 0, err
	}

	var raw string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(content_json::text, '')
		FROM run_artifacts
		WHERE run_id = $1 AND artifact_kind = 'attempt_state'
		LIMIT 1
	`, runID).Scan(&raw)

	current := 0
	if err == nil && strings.TrimSpace(raw) != "" {
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(raw), &obj); jerr == nil {
			if v, ok := obj["attempt"]; ok {
				switch t := v.(type) {
				case float64:
					current = int(t)
				case int:
					current = t
				case int64:
					current = int(t)
				case string:
					if n, e := strconv.Atoi(t); e == nil {
						current = n
					}
				}
			}
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	nextAttempt := current + 1
	attemptState := map[string]any{
		"attempt":    nextAttempt,
		"updated_at": NowRFC3339Nano(),
	}

	b, jerr := json.Marshal(attemptState)
	if jerr != nil {
		b = []byte(`{}`)
	}

	runIDText := runID

	_, err = tx.Exec(ctx, `
		INSERT INTO run_artifacts(
			run_id,
			artifact_kind,
			content_json,
			created_at,
			updated_at,
			schema_version,
			trace_id,
			artifact_ref_kind,
			artifact_ref_run_id,
			artifact_ref_trace_id,
			trace_trace_id
		)
		VALUES (
			$1,
			'attempt_state',
			$2::jsonb,
			now(),
			now(),
			$3,
			$4,
			'attempt_state',
			$5,
			$4,
			$4
		)
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET
			content_json = EXCLUDED.content_json,
			updated_at  = now(),
			schema_version        = EXCLUDED.schema_version,
			trace_id              = EXCLUDED.trace_id,
			artifact_ref_kind     = EXCLUDED.artifact_ref_kind,
			artifact_ref_run_id   = EXCLUDED.artifact_ref_run_id,
			artifact_ref_trace_id = EXCLUDED.artifact_ref_trace_id,
			trace_trace_id        = EXCLUDED.trace_trace_id
	`, runID, string(b), RunArtifactSchemaVersion, traceID, runIDText)
	if err != nil {
		return 0, err
	}

	payload := map[string]any{
		"attempt":    nextAttempt,
		"project_id": projectID,
		"worker_id":  workerID,
		"started_at": NowRFC3339Nano(),
	}
	pb := MarshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, seq, "run.attempt_started", pb)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET next_event_seq=$2, updated_at=now()
		WHERE run_id=$1
	`, runID, seq+1)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return nextAttempt, nil
}

func (s *Store) InsertLearnSignal(ctx context.Context, runID, projectID, signalType string, payload map[string]any) error {
	runID = NormalizeRunID(runID)
	projectID = strings.TrimSpace(projectID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pb := MarshalJSONOrEmptyMap(payload)

	_, err := s.db.Exec(ctx, `
		INSERT INTO learn_signals(run_id, project_id, signal_type, payload)
		VALUES ($1,$2,$3,$4::jsonb)
	`, runID, projectID, signalType, pb)
	return err
}