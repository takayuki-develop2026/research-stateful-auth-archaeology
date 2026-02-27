package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type DiscoverySourceRepository struct{ db *sql.DB }

func NewDiscoverySourceRepository(db *sql.DB) *DiscoverySourceRepository {
	return &DiscoverySourceRepository{db: db}
}

// Implements run.DiscoverySourceRepo
func (r *DiscoverySourceRepository) Upsert(
	ctx context.Context,
	in run.DiscoverySourceUpsertInput,
) (run.DiscoverySourceUpsertResult, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.SourceRefRaw = strings.TrimSpace(in.SourceRefRaw)
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	in.SourceHash = strings.TrimSpace(in.SourceHash)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" || in.PipelineVersion == "" || in.PolicyVersion == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("project_id/run_id/trace_id/pipeline_version/policy_version are required")
	}
	if in.SourceType == "" || in.SourceRefRaw == "" || in.SourceRef == "" || len(in.SourceHash) != 64 {
		return run.DiscoverySourceUpsertResult{}, errors.New("source_type/source_ref_raw/source_ref/source_hash(64) are required")
	}

	var st *string
	if in.Status != nil {
		v := string(*in.Status)
		st = &v
	}
	var fs *string
	if in.FailureState != nil {
		v := string(*in.FailureState)
		fs = &v
	}

	const q = `
WITH existing AS (
  SELECT id FROM public.discovery_sources
  WHERE project_id=$1 AND source_type=$2 AND source_hash=$3
  LIMIT 1
),
upsert AS (
  INSERT INTO public.discovery_sources(
    project_id, source_type, source_ref_raw, source_ref, source_hash,
    run_id, trace_id, pipeline_version, policy_version,
    status, failure_state, failure_code, failure_message,
    first_seen_at, last_seen_at, seen_count,
    created_at, updated_at
  )
  VALUES (
    $1,$2,$4,$5,$3,
    $6::uuid,$7,$8,$9,
    COALESCE($10,'detected'),
    COALESCE($11,'none'),
    $12, $13,
    now(), now(), 1,
    now(), now()
  )
  ON CONFLICT (project_id, source_type, source_hash) DO UPDATE
    SET source_ref_raw = EXCLUDED.source_ref_raw,
        source_ref = EXCLUDED.source_ref,
        run_id = EXCLUDED.run_id,
        trace_id = EXCLUDED.trace_id,
        pipeline_version = EXCLUDED.pipeline_version,
        policy_version = EXCLUDED.policy_version,
        status = COALESCE($10, public.discovery_sources.status),
        failure_state = COALESCE($11, public.discovery_sources.failure_state),
        failure_code = COALESCE($12, public.discovery_sources.failure_code),
        failure_message = COALESCE($13, public.discovery_sources.failure_message),
        last_seen_at = now(),
        seen_count = public.discovery_sources.seen_count + 1,
        updated_at = now()
  RETURNING id
)
SELECT
  (SELECT id FROM upsert) AS source_id,
  EXISTS(SELECT 1 FROM existing) AS found_existing;
`

	var out run.DiscoverySourceUpsertResult
	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID, in.SourceType, in.SourceHash,
		in.SourceRefRaw, in.SourceRef,
		in.RunID, in.TraceID, in.PipelineVersion, in.PolicyVersion,
		st, fs, in.FailureCode, in.FailureMsg,
	).Scan(&out.SourceID, &out.FoundExisting); err != nil {
		return run.DiscoverySourceUpsertResult{}, err
	}
	return out, nil
}

func (r *DiscoverySourceRepository) Get(
	ctx context.Context,
	projectID string,
	sourceID int64,
) (run.DiscoverySource, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || sourceID <= 0 {
		return run.DiscoverySource{}, errors.New("project_id and source_id are required")
	}

	const q = `
SELECT id, project_id, run_id::text, trace_id, pipeline_version, policy_version,
       source_type, source_ref_raw, source_ref, source_hash,
       status, failure_state, failure_code, failure_message,
       first_seen_at, last_seen_at, seen_count
FROM public.discovery_sources
WHERE project_id=$1 AND id=$2
LIMIT 1;
`
	var s run.DiscoverySource
	var status, fail string
	var fcode, fmsg sql.NullString

	if err := r.db.QueryRowContext(ctx, q, projectID, sourceID).Scan(
		&s.ID, &s.ProjectID, &s.RunID, &s.TraceID, &s.PipelineVersion, &s.PolicyVersion,
		&s.SourceType, &s.SourceRefRaw, &s.SourceRef, &s.SourceHash,
		&status, &fail, &fcode, &fmsg,
		&s.FirstSeenAt, &s.LastSeenAt, &s.SeenCount,
	); err != nil {
		return run.DiscoverySource{}, err
	}
	s.Status = run.SourceStatus(status)
	s.FailureState = run.SourceFailureState(fail)

	if fcode.Valid {
		v := fcode.String
		s.FailureCode = &v
	}
	if fmsg.Valid {
		v := fmsg.String
		s.FailureMsg = &v
	}
	return s, nil
}
