package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"example.com/pisag_go/run"
)

type LifecycleRepository struct{ db *sql.DB }

func NewLifecycleRepository(db *sql.DB) *LifecycleRepository {
	return &LifecycleRepository{db: db}
}

func sha256hexLC(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (r *LifecycleRepository) RunJob(ctx context.Context, in run.LifecycleJobRunInput) (run.LifecycleJobRunResult, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.JobType = strings.TrimSpace(in.JobType)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" || in.JobType == "" {
		return run.LifecycleJobRunResult{}, errors.New("project_id/run_id/trace_id/job_type are required")
	}
	if in.Limit <= 0 || in.Limit > 5000 {
		in.Limit = 200
	}

	jobKey := sha256hexLC(
		"v87|" + in.ProjectID + "|" + in.JobType + "|" + in.RunID +
			"|dry=" + bool01(in.DryRun) + "|limit=" + strconv.Itoa(in.Limit),
	)

	// 1) create job row (idempotent)
	jobID, err := r.upsertJob(ctx, in.ProjectID, in.JobType, jobKey, in.TraceID, in.RunID)
	if err != nil {
		return run.LifecycleJobRunResult{}, err
	}

	// 2) run job in tx (updates + events)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
		return run.LifecycleJobRunResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res := run.LifecycleJobRunResult{JobID: jobID}

	switch in.JobType {
	case "mark_stale":
		scanned, changed, err := r.jobMarkStale(ctx, tx, in.ProjectID, in.RunID, in.TraceID, in.Limit, in.DryRun)
		if err != nil {
			_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
			return run.LifecycleJobRunResult{}, err
		}
		res.Scanned, res.Changed = scanned, changed

	case "archive_expired":
		scanned, archived, err := r.jobArchiveExpired(ctx, tx, in.ProjectID, in.RunID, in.TraceID, in.Limit, in.DryRun)
		if err != nil {
			_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
			return run.LifecycleJobRunResult{}, err
		}
		res.Scanned, res.Archived = scanned, archived
		res.Changed = archived

	case "schedule_retry":
		scanned, changed, err := r.jobScheduleRetry(ctx, tx, in.ProjectID, in.RunID, in.TraceID, in.Limit, in.DryRun)
		if err != nil {
			_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
			return run.LifecycleJobRunResult{}, err
		}
		res.Scanned, res.Changed = scanned, changed

	case "schedule_apply_retry":
		scanned, changed, err := r.jobScheduleApplyRetry(ctx, tx, in.ProjectID, in.RunID, in.TraceID, in.Limit, in.DryRun)
		if err != nil {
			_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
			return run.LifecycleJobRunResult{}, err
		}
		res.Scanned, res.Changed = scanned, changed

	case "requeue_review":
		scanned, requeued, err := r.jobRequeueReview(ctx, tx, in.ProjectID, in.RunID, in.TraceID, in.Limit, in.DryRun)
		if err != nil {
			_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
			return run.LifecycleJobRunResult{}, err
		}
		res.Scanned, res.Requeued = scanned, requeued
		res.Changed = requeued

	default:
		_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": "unknown job_type"})
		return run.LifecycleJobRunResult{}, errors.New("unknown job_type")
	}

	if err := tx.Commit(); err != nil {
		_ = r.finishJob(ctx, in.ProjectID, jobID, "failed", map[string]any{"error": err.Error()})
		return run.LifecycleJobRunResult{}, err
	}

	// 3) finish job row
	_ = r.finishJob(ctx, in.ProjectID, jobID, "done", map[string]any{
		"job_type": in.JobType,
		"dry_run":  in.DryRun,
		"limit":    in.Limit,
		"scanned":  res.Scanned,
		"changed":  res.Changed,
		"archived": res.Archived,
		"requeued": res.Requeued,
	})

	return res, nil
}

// ------------------------------------------------------------
// job row helpers
// ------------------------------------------------------------

func (r *LifecycleRepository) upsertJob(ctx context.Context, projectID, jobType, jobKey, traceID, runID string) (int64, error) {
	const q = `
INSERT INTO public.discovery_lifecycle_jobs(
  project_id, job_type, job_key, status, stats, trace_id, run_id, started_at, created_at
) VALUES (
  $1,$2,$3,'running','{}'::jsonb,$4,$5::uuid,now(),now()
)
ON CONFLICT (project_id, job_key) DO UPDATE
  SET status='running',
      trace_id=EXCLUDED.trace_id,
      run_id=EXCLUDED.run_id,
      started_at=now()
RETURNING id;
`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, projectID, jobType, jobKey, traceID, runID).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *LifecycleRepository) finishJob(ctx context.Context, projectID string, jobID int64, status string, stats map[string]any) error {
	b, _ := json.Marshal(stats)
	const q = `
UPDATE public.discovery_lifecycle_jobs
SET status=$3,
    stats=$4::jsonb,
    finished_at=now()
WHERE project_id=$1 AND id=$2;
`
	_, err := r.db.ExecContext(ctx, q, projectID, jobID, status, string(b))
	return err
}

// ------------------------------------------------------------
// job implementations (TX)
// ------------------------------------------------------------

func (r *LifecycleRepository) jobMarkStale(ctx context.Context, tx *sql.Tx, projectID, runID, traceID string, limit int, dryRun bool) (int64, int64, error) {
	if dryRun {
		const q = `
SELECT count(*)
FROM public.discovery_candidates
WHERE project_id=$1
  AND archived_at IS NULL
  AND status='review_required'
  AND stale_at IS NULL
  AND review_requested_at IS NOT NULL
  AND review_requested_at < now() - interval '7 days';
`
		var n int64
		if err := tx.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > int64(limit) {
			n = int64(limit)
		}
		return n, 0, nil
	}

	const q = `
WITH tgt AS (
  SELECT id
  FROM public.discovery_candidates
  WHERE project_id=$1
    AND archived_at IS NULL
    AND status='review_required'
    AND stale_at IS NULL
    AND review_requested_at IS NOT NULL
    AND review_requested_at < now() - interval '7 days'
  ORDER BY review_requested_at ASC
  LIMIT $2
),
upd AS (
  UPDATE public.discovery_candidates c
  SET stale_at = now(),
      updated_at = now()
  FROM tgt
  WHERE c.id=tgt.id
  RETURNING c.id
),
evt AS (
  INSERT INTO public.discovery_candidate_lifecycle_events(
    project_id, candidate_id, event_type, actor_type, actor_id,
    message, detail_evidence_ref, trace_id, run_id, created_at
  )
  SELECT $1, id, 'stale_marked', 'system', NULL,
         'marked stale by lifecycle job', NULL, $3, $4::uuid, now()
  FROM upd
)
SELECT (SELECT count(*) FROM tgt) AS scanned,
       (SELECT count(*) FROM upd) AS changed;
`
	var scanned, changed int64
	if err := tx.QueryRowContext(ctx, q, projectID, limit, traceID, runID).Scan(&scanned, &changed); err != nil {
		return 0, 0, err
	}
	return scanned, changed, nil
}

func (r *LifecycleRepository) jobArchiveExpired(ctx context.Context, tx *sql.Tx, projectID, runID, traceID string, limit int, dryRun bool) (int64, int64, error) {
	if dryRun {
		const q = `
SELECT count(*)
FROM public.discovery_candidates
WHERE project_id=$1
  AND archived_at IS NULL
  AND expires_at IS NOT NULL
  AND expires_at <= now();
`
		var n int64
		if err := tx.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > int64(limit) {
			n = int64(limit)
		}
		return n, 0, nil
	}

	const q = `
WITH tgt AS (
  SELECT id
  FROM public.discovery_candidates
  WHERE project_id=$1
    AND archived_at IS NULL
    AND expires_at IS NOT NULL
    AND expires_at <= now()
  ORDER BY expires_at ASC
  LIMIT $2
),
upd AS (
  UPDATE public.discovery_candidates c
  SET archived_at = now(),
      archive_reason = 'expired',
      updated_at = now()
  FROM tgt
  WHERE c.id=tgt.id
  RETURNING c.id
),
evt AS (
  INSERT INTO public.discovery_candidate_lifecycle_events(
    project_id, candidate_id, event_type, actor_type, actor_id,
    message, detail_evidence_ref, trace_id, run_id, created_at
  )
  SELECT $1, id, 'archived', 'system', NULL,
         'archived by expiry', NULL, $3, $4::uuid, now()
  FROM upd
)
SELECT (SELECT count(*) FROM tgt) AS scanned,
       (SELECT count(*) FROM upd) AS archived;
`
	var scanned, archived int64
	if err := tx.QueryRowContext(ctx, q, projectID, limit, traceID, runID).Scan(&scanned, &archived); err != nil {
		return 0, 0, err
	}
	return scanned, archived, nil
}

func (r *LifecycleRepository) jobScheduleRetry(ctx context.Context, tx *sql.Tx, projectID, runID, traceID string, limit int, dryRun bool) (int64, int64, error) {
	if dryRun {
		const q = `
SELECT count(*)
FROM public.discovery_candidates
WHERE project_id=$1
  AND archived_at IS NULL
  AND status='needs_retry'
  AND (retry_next_at IS NULL OR retry_next_at <= now())
  AND retry_attempts < 8;
`
		var n int64
		if err := tx.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > int64(limit) {
			n = int64(limit)
		}
		return n, 0, nil
	}

	const q = `
WITH tgt AS (
  SELECT id
  FROM public.discovery_candidates
  WHERE project_id=$1
    AND archived_at IS NULL
    AND status='needs_retry'
    AND (retry_next_at IS NULL OR retry_next_at <= now())
    AND retry_attempts < 8
  ORDER BY last_seen_at DESC
  LIMIT $2
),
upd AS (
  UPDATE public.discovery_candidates c
  SET retry_attempts = c.retry_attempts + 1,
      retry_backoff_sec = LEAST(900 * CAST(power(2, c.retry_attempts) AS int), 86400),
      retry_next_at = now() + (LEAST(900 * CAST(power(2, c.retry_attempts) AS int), 86400) || ' seconds')::interval,
      updated_at = now()
  FROM tgt
  WHERE c.id=tgt.id
  RETURNING c.id
),
evt AS (
  INSERT INTO public.discovery_candidate_lifecycle_events(
    project_id, candidate_id, event_type, actor_type, actor_id,
    message, detail_evidence_ref, trace_id, run_id, created_at
  )
  SELECT $1, id, 'retry_scheduled', 'system', NULL,
         'retry scheduled by lifecycle job', NULL, $3, $4::uuid, now()
  FROM upd
)
SELECT (SELECT count(*) FROM tgt) AS scanned,
       (SELECT count(*) FROM upd) AS changed;
`
	var scanned, changed int64
	if err := tx.QueryRowContext(ctx, q, projectID, limit, traceID, runID).Scan(&scanned, &changed); err != nil {
		return 0, 0, err
	}
	return scanned, changed, nil
}

func (r *LifecycleRepository) jobScheduleApplyRetry(ctx context.Context, tx *sql.Tx, projectID, runID, traceID string, limit int, dryRun bool) (int64, int64, error) {
	if dryRun {
		const q = `
SELECT count(*)
FROM public.discovery_candidates
WHERE project_id=$1
  AND archived_at IS NULL
  AND status='approved'
  AND (apply_next_at IS NULL OR apply_next_at <= now())
  AND apply_attempts < 8;
`
		var n int64
		if err := tx.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > int64(limit) {
			n = int64(limit)
		}
		return n, 0, nil
	}

	const q = `
WITH tgt AS (
  SELECT id
  FROM public.discovery_candidates
  WHERE project_id=$1
    AND archived_at IS NULL
    AND status='approved'
    AND (apply_next_at IS NULL OR apply_next_at <= now())
    AND apply_attempts < 8
  ORDER BY last_seen_at DESC
  LIMIT $2
),
upd AS (
  UPDATE public.discovery_candidates c
  SET apply_attempts = c.apply_attempts + 1,
      apply_next_at = now() + (LEAST(900 * CAST(power(2, c.apply_attempts) AS int), 86400) || ' seconds')::interval,
      updated_at = now()
  FROM tgt
  WHERE c.id=tgt.id
  RETURNING c.id
),
evt AS (
  INSERT INTO public.discovery_candidate_lifecycle_events(
    project_id, candidate_id, event_type, actor_type, actor_id,
    message, detail_evidence_ref, trace_id, run_id, created_at
  )
  SELECT $1, id, 'apply_retry_scheduled', 'system', NULL,
         'apply retry scheduled by lifecycle job', NULL, $3, $4::uuid, now()
  FROM upd
)
SELECT (SELECT count(*) FROM tgt) AS scanned,
       (SELECT count(*) FROM upd) AS changed;
`
	var scanned, changed int64
	if err := tx.QueryRowContext(ctx, q, projectID, limit, traceID, runID).Scan(&scanned, &changed); err != nil {
		return 0, 0, err
	}
	return scanned, changed, nil
}

func (r *LifecycleRepository) jobRequeueReview(ctx context.Context, tx *sql.Tx, projectID, runID, traceID string, limit int, dryRun bool) (int64, int64, error) {
	if dryRun {
		const q = `
SELECT count(*)
FROM public.discovery_candidates
WHERE project_id=$1
  AND archived_at IS NULL
  AND status='review_required'
  AND stale_at IS NOT NULL;
`
		var n int64
		if err := tx.QueryRowContext(ctx, q, projectID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > int64(limit) {
			n = int64(limit)
		}
		return n, 0, nil
	}

	const q = `
WITH tgt AS (
  SELECT id
  FROM public.discovery_candidates
  WHERE project_id=$1
    AND archived_at IS NULL
    AND status='review_required'
    AND stale_at IS NOT NULL
  ORDER BY stale_at ASC
  LIMIT $2
),
upd AS (
  UPDATE public.discovery_candidates c
  SET review_requested_at = now(),
      stale_at = NULL,
      updated_at = now()
  FROM tgt
  WHERE c.id=tgt.id
  RETURNING c.id
),
evt AS (
  INSERT INTO public.discovery_candidate_lifecycle_events(
    project_id, candidate_id, event_type, actor_type, actor_id,
    message, detail_evidence_ref, trace_id, run_id, created_at
  )
  SELECT $1, id, 'review_requeued', 'system', NULL,
         'requeued review by lifecycle job', NULL, $3, $4::uuid, now()
  FROM upd
)
SELECT (SELECT count(*) FROM tgt) AS scanned,
       (SELECT count(*) FROM upd) AS requeued;
`
	var scanned, requeued int64
	if err := tx.QueryRowContext(ctx, q, projectID, limit, traceID, runID).Scan(&scanned, &requeued); err != nil {
		return 0, 0, err
	}
	return scanned, requeued, nil
}

// ------------------------------------------------------------
// helpers
// ------------------------------------------------------------

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
