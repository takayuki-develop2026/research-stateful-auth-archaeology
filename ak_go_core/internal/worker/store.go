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

// PickQueuedRun claims a single queued run and transitions it to running.
//
// DB truth (public.runs):
// - run_id uuid PK
// - trace_id uuid NOT NULL
// - status run_status (queued|created|running|done|failed)
// - NO "state" column
//
// Contract:
// - Never throw from DB layer: caller decides error policy.
// - Use SKIP LOCKED to allow concurrent workers.
func (s *Store) PickQueuedRun(ctx context.Context, workerID string) (RunRow, bool, error) {
	_ = workerID // future: picked_by in run_artifacts or run_events

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RunRow{}, false, err
	}
	defer tx.Rollback(ctx)

	var r RunRow
	err = tx.QueryRow(ctx, `
		SELECT run_id::text, trace_id::text
		FROM runs
		WHERE status = 'queued'
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

	// trace_id should always be present (DB NOT NULL), but guard anyway.
	if r.TraceID == "" {
		err = tx.QueryRow(ctx, `
			UPDATE runs
			SET trace_id = gen_random_uuid(),
			    updated_at = now()
			WHERE run_id = $1::uuid
			RETURNING trace_id::text
		`, r.RunID).Scan(&r.TraceID)
		if err != nil {
			return RunRow{}, false, err
		}
		r.TraceID = strings.TrimSpace(r.TraceID)
	}

	// Transition queued -> running (idempotent check)
	tag, err := tx.Exec(ctx, `
		UPDATE runs
		SET status = 'running',
		    started_at = now(),
		    updated_at = now()
		WHERE run_id = $1::uuid
		  AND status = 'queued'
	`, r.RunID)
	if err != nil {
		return RunRow{}, false, err
	}
	if tag.RowsAffected() != 1 {
		// Lost race (someone else updated) or run not queued anymore.
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
		WHERE run_id = $1::uuid
	`, runID).Scan(&projectID)
	return strings.TrimSpace(projectID), err
}

func (s *Store) GetRunStatus(ctx context.Context, runID string) (string, error) {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var st string
	err := s.db.QueryRow(ctx, `
		SELECT status::text
		FROM runs
		WHERE run_id=$1::uuid
	`, runID).Scan(&st)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(st), nil
}

// Kept for backward call sites (old name).
func (s *Store) GetRunState(ctx context.Context, runID string) (string, error) {
	return s.GetRunStatus(ctx, runID)
}

func (s *Store) GetRunModeFromEnqueuedEvent(ctx context.Context, runID string) (int, error) {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var modeText string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(payload->>'mode', '')
		FROM run_events
		WHERE run_id = $1::uuid AND event_name = 'run.enqueued'
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

// AppendEventAndStatus updates runs.status to a new run_status value and writes a run_event.
// newStatus must be a valid enum value in run_status.
func (s *Store) AppendEventAndStatus(ctx context.Context, runID, traceID, eventName, newStatus string, payload map[string]any) error {
	return s.appendEventInternal(ctx, runID, traceID, eventName, payload, newStatus, true)
}

func (s *Store) appendEventInternal(
	ctx context.Context,
	runID, traceID, eventName string,
	payload map[string]any,
	newStatus string,
	updateStatus bool,
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

	// Ensure we have a trace_id (uuid text).
	var dbTraceID string
	err = tx.QueryRow(ctx, `
		SELECT trace_id::text
		FROM runs
		WHERE run_id=$1::uuid
		FOR UPDATE
	`, runID).Scan(&dbTraceID)
	if err != nil {
		return err
	}
	dbTraceID = strings.TrimSpace(dbTraceID)
	if traceID == "" {
		traceID = dbTraceID
	}

	seq, err := nextEventSeqTx(ctx, tx, runID)
	if err != nil {
		return err
	}

	pb := MarshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
	`, runID, traceID, seq, eventName, pb)
	if err != nil {
		return err
	}

	if !updateStatus {
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET updated_at=now()
			WHERE run_id=$1::uuid
		`, runID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// Update status and timestamps.
	// - done => finished_at set
	// - failed => finished_at set
	// - others => no finished_at
	var tag pgconn.CommandTag
	switch newStatus {
	case "done", "failed":
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET status=$2::run_status,
			    finished_at=now(),
			    updated_at=now()
			WHERE run_id=$1::uuid
		`, runID, newStatus)
	default:
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET status=$2::run_status,
			    updated_at=now()
			WHERE run_id=$1::uuid
		`, runID, newStatus)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("runs update affected=%d (run_id=%s)", tag.RowsAffected(), runID)
	}

	return tx.Commit(ctx)
}

// nextEventSeqTx allocates next event_seq as max(event_seq)+1.
// runs has no next_event_seq column in current DB.
func nextEventSeqTx(ctx context.Context, tx pgx.Tx, runID string) (int64, error) {
	runID = NormalizeRunID(runID)

	var maxSeq int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0)
		FROM run_events
		WHERE run_id=$1::uuid
	`, runID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
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

	runIDText := runID

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
			$1::uuid,
			$2,
			$3::jsonb,
			now(),
			now(),
			$4,
			$5::uuid,
			$2,
			$6,
			$5::uuid,
			$5::uuid
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
		WHERE run_id = $1::uuid AND artifact_kind = 'attempt_state'
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
			$1::uuid,
			'attempt_state',
			$2::jsonb,
			now(),
			now(),
			$3,
			$4::uuid,
			'attempt_state',
			$5,
			$4::uuid,
			$4::uuid
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
		VALUES ($1::uuid,$2::uuid,$3,$4,$5::jsonb)
	`, runID, traceID, seq, "run.attempt_started", pb)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET updated_at=now()
		WHERE run_id=$1::uuid
	`, runID)
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
		VALUES ($1::uuid,$2,$3,$4::jsonb)
	`, runID, projectID, signalType, pb)
	return err
}

func (s *Store) MarkDone(ctx context.Context, runID string) error {
	runID = NormalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		UPDATE runs
		SET status='done',
		    finished_at=now(),
		    updated_at=now()
		WHERE run_id=$1::uuid
	`, runID)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, runID, errorCode, errorMessage string) error {
	runID = NormalizeRunID(runID)
	errorCode = strings.TrimSpace(errorCode)
	errorMessage = strings.TrimSpace(errorMessage)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		UPDATE runs
		SET status='failed',
		    error_code=$2,
		    error_message=$3,
		    finished_at=now(),
		    updated_at=now()
		WHERE run_id=$1::uuid
	`, runID, errorCode, errorMessage)
	return err
}