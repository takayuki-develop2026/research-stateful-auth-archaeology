package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type DiscoveryRepository struct{ db *sql.DB }

func NewDiscoveryRepository(db *sql.DB) *DiscoveryRepository {
	return &DiscoveryRepository{db: db}
}

func (r *DiscoveryRepository) UpsertSource(ctx context.Context, in run.DiscoverySourceUpsertInput) (run.DiscoverySourceUpsertResult, error) {
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

	// NOTE: This is table-direct (v8 grants). For EXECUTE ONLY later, replace with fn.
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
    first_seen_at, last_seen_at, seen_count
  )
  VALUES (
    $1,$2,$4,$5,$3,
    $6::uuid,$7,$8,$9,
    COALESCE($10,'detected'), COALESCE($11,'none'), $12, $13,
    now(), now(), 1
  )
  ON CONFLICT (project_id, source_type, source_hash) DO UPDATE
    SET source_ref_raw = EXCLUDED.source_ref_raw,
        source_ref = EXCLUDED.source_ref,
        run_id = EXCLUDED.run_id,
        trace_id = EXCLUDED.trace_id,
        pipeline_version = EXCLUDED.pipeline_version,
        policy_version = EXCLUDED.policy_version,
        -- progress/failure are optional; keep existing if NULL
        status = COALESCE($10, public.discovery_sources.status),
        failure_state = COALESCE($11, public.discovery_sources.failure_state),
        failure_code = COALESCE($12, public.discovery_sources.failure_code),
        failure_message = COALESCE($13, public.discovery_sources.failure_message),
        last_seen_at = now(),
        seen_count = public.discovery_sources.seen_count + 1
  RETURNING id
)
SELECT
  (SELECT id FROM upsert) AS source_id,
  EXISTS(SELECT 1 FROM existing) AS found_existing;
`

	var st *string
	var fs *string
	if in.Status != nil {
		v := string(*in.Status)
		st = &v
	}
	if in.FailureState != nil {
		v := string(*in.FailureState)
		fs = &v
	}

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

func (r *DiscoveryRepository) GetSource(ctx context.Context, projectID string, sourceID int64) (run.DiscoverySource, error) {
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
	if err := r.db.QueryRowContext(ctx, q, projectID, sourceID).Scan(
		&s.ID, &s.ProjectID, &s.RunID, &s.TraceID, &s.PipelineVersion, &s.PolicyVersion,
		&s.SourceType, &s.SourceRefRaw, &s.SourceRef, &s.SourceHash,
		&status, &fail, &s.FailureCode, &s.FailureMsg,
		&s.FirstSeenAt, &s.LastSeenAt, &s.SeenCount,
	); err != nil {
		return run.DiscoverySource{}, err
	}
	s.Status = run.SourceStatus(status)
	s.FailureState = run.SourceFailureState(fail)
	return s, nil
}

func (r *DiscoveryRepository) UpsertCandidate(ctx context.Context, in run.DiscoveryCandidateUpsertInput) (run.DiscoveryCandidateUpsertResult, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.CandidateType = strings.TrimSpace(in.CandidateType)
	in.CandidateKey = strings.TrimSpace(in.CandidateKey)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" || in.PipelineVersion == "" || in.PolicyVersion == "" {
		return run.DiscoveryCandidateUpsertResult{}, errors.New("project_id/run_id/trace_id/pipeline_version/policy_version are required")
	}
	if in.SourceID <= 0 || in.CandidateType == "" || len(in.CandidateKey) != 64 {
		return run.DiscoveryCandidateUpsertResult{}, errors.New("source_id/candidate_type/candidate_key(64) are required")
	}

	var st *string
	if in.Status != nil {
		v := string(*in.Status)
		st = &v
	}
	var rl *string
	if in.RiskLevel != nil {
		v := string(*in.RiskLevel)
		rl = &v
	}

	const q = `
WITH existing AS (
  SELECT id FROM public.discovery_candidates
  WHERE project_id=$1 AND candidate_type=$2 AND candidate_key=$3
  LIMIT 1
),
upsert AS (
  INSERT INTO public.discovery_candidates(
    project_id, source_id,
    candidate_type, candidate_key,
    status, risk_level, confidence,
    payload_evidence_ref, normalized_evidence_ref, diff_evidence_ref,
    run_id, trace_id, pipeline_version, policy_version,
    first_seen_at, last_seen_at, seen_count
  )
  VALUES (
    $1,$4,
    $2,$3,
    COALESCE($5,'proposed'),
    COALESCE($6,'normal'),
    $7,
    $8::uuid, $9::uuid, $10::uuid,
    $11::uuid, $12, $13, $14,
    now(), now(), 1
  )
  ON CONFLICT (project_id, candidate_type, candidate_key) DO UPDATE
    SET source_id = EXCLUDED.source_id,
        run_id = EXCLUDED.run_id,
        trace_id = EXCLUDED.trace_id,
        pipeline_version = EXCLUDED.pipeline_version,
        policy_version = EXCLUDED.policy_version,
        status = COALESCE($5, public.discovery_candidates.status),
        risk_level = COALESCE($6, public.discovery_candidates.risk_level),
        confidence = COALESCE($7, public.discovery_candidates.confidence),
        payload_evidence_ref = COALESCE($8::uuid, public.discovery_candidates.payload_evidence_ref),
        normalized_evidence_ref = COALESCE($9::uuid, public.discovery_candidates.normalized_evidence_ref),
        diff_evidence_ref = COALESCE($10::uuid, public.discovery_candidates.diff_evidence_ref),
        last_seen_at = now(),
        seen_count = public.discovery_candidates.seen_count + 1
  RETURNING id
)
SELECT
  (SELECT id FROM upsert) AS candidate_id,
  EXISTS(SELECT 1 FROM existing) AS found_existing;
`

	var out run.DiscoveryCandidateUpsertResult
	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID, in.CandidateType, in.CandidateKey, in.SourceID,
		st, rl, in.Confidence,
		in.PayloadEvidenceRef, in.NormalizedEvidenceRef, in.DiffEvidenceRef,
		in.RunID, in.TraceID, in.PipelineVersion, in.PolicyVersion,
	).Scan(&out.CandidateID, &out.FoundExisting); err != nil {
		return run.DiscoveryCandidateUpsertResult{}, err
	}
	return out, nil
}

func (r *DiscoveryRepository) GetCandidate(ctx context.Context, projectID string, candidateID int64) (run.DiscoveryCandidate, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || candidateID <= 0 {
		return run.DiscoveryCandidate{}, errors.New("project_id and candidate_id are required")
	}

	const q = `
SELECT id, project_id, source_id,
       candidate_type, candidate_key, status, risk_level, confidence,
       payload_evidence_ref::text, normalized_evidence_ref::text, diff_evidence_ref::text,
       dedupe_key, dedupe_group_id,
       first_seen_at, last_seen_at, seen_count,
       review_requested_at, decided_at,
       run_id::text, trace_id, pipeline_version, policy_version
FROM public.discovery_candidates
WHERE project_id=$1 AND id=$2
LIMIT 1;
`

	var c run.DiscoveryCandidate
	var status, risk string
	var payloadRef, normRef, diffRef sql.NullString
	var dedupeKey sql.NullString
	var dedupeGroup sql.NullInt64

	if err := r.db.QueryRowContext(ctx, q, projectID, candidateID).Scan(
		&c.ID, &c.ProjectID, &c.SourceID,
		&c.CandidateType, &c.CandidateKey, &status, &risk, &c.Confidence,
		&payloadRef, &normRef, &diffRef,
		&dedupeKey, &dedupeGroup,
		&c.FirstSeenAt, &c.LastSeenAt, &c.SeenCount,
		&c.ReviewRequestedAt, &c.DecidedAt,
		&c.RunID, &c.TraceID, &c.PipelineVersion, &c.PolicyVersion,
	); err != nil {
		return run.DiscoveryCandidate{}, err
	}
	c.Status = run.CandidateStatus(status)
	c.RiskLevel = run.RiskLevel(risk)

	if payloadRef.Valid {
		v := payloadRef.String
		c.PayloadEvidenceRef = &v
	}
	if normRef.Valid {
		v := normRef.String
		c.NormalizedEvidenceRef = &v
	}
	if diffRef.Valid {
		v := diffRef.String
		c.DiffEvidenceRef = &v
	}
	if dedupeKey.Valid {
		v := dedupeKey.String
		c.DedupeKey = &v
	}
	if dedupeGroup.Valid {
		v := dedupeGroup.Int64
		c.DedupeGroupID = &v
	}
	return c, nil
}

func (r *DiscoveryRepository) ListCandidatesOps(ctx context.Context, projectID string, mode string, limit int) ([]run.DiscoveryCandidate, error) {
	projectID = strings.TrimSpace(projectID)
	mode = strings.TrimSpace(mode)
	if projectID == "" || mode == "" {
		return nil, errors.New("project_id and mode are required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	where := ""
	switch mode {
	case "stale":
		where = "stale_at IS NOT NULL AND archived_at IS NULL"
	case "retry":
		where = "retry_next_at IS NOT NULL AND archived_at IS NULL AND retry_next_at <= now()"
	case "apply_retry":
		where = "apply_next_at IS NOT NULL AND archived_at IS NULL AND apply_next_at <= now()"
	case "archived":
		where = "archived_at IS NOT NULL"
	default:
		return nil, errors.New("mode must be stale|retry|apply_retry|archived")
	}

	q := `
SELECT id, project_id, source_id,
       candidate_type, candidate_key, status, risk_level, confidence,
       payload_evidence_ref::text, normalized_evidence_ref::text, diff_evidence_ref::text,
       dedupe_key, dedupe_group_id,
       first_seen_at, last_seen_at, seen_count,
       review_requested_at, decided_at,
       run_id::text, trace_id, pipeline_version, policy_version
FROM public.discovery_candidates
WHERE project_id=$1 AND ` + where + `
ORDER BY last_seen_at DESC
LIMIT $2;
`
	rows, err := r.db.QueryContext(ctx, q, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []run.DiscoveryCandidate
	for rows.Next() {
		var c run.DiscoveryCandidate
		var status, risk string
		var payloadRef, normRef, diffRef sql.NullString
		var dedupeKey sql.NullString
		var dedupeGroup sql.NullInt64

		if err := rows.Scan(
			&c.ID, &c.ProjectID, &c.SourceID,
			&c.CandidateType, &c.CandidateKey, &status, &risk, &c.Confidence,
			&payloadRef, &normRef, &diffRef,
			&dedupeKey, &dedupeGroup,
			&c.FirstSeenAt, &c.LastSeenAt, &c.SeenCount,
			&c.ReviewRequestedAt, &c.DecidedAt,
			&c.RunID, &c.TraceID, &c.PipelineVersion, &c.PolicyVersion,
		); err != nil {
			return nil, err
		}
		c.Status = run.CandidateStatus(status)
		c.RiskLevel = run.RiskLevel(risk)

		if payloadRef.Valid {
			v := payloadRef.String
			c.PayloadEvidenceRef = &v
		}
		if normRef.Valid {
			v := normRef.String
			c.NormalizedEvidenceRef = &v
		}
		if diffRef.Valid {
			v := diffRef.String
			c.DiffEvidenceRef = &v
		}
		if dedupeKey.Valid {
			v := dedupeKey.String
			c.DedupeKey = &v
		}
		if dedupeGroup.Valid {
			v := dedupeGroup.Int64
			c.DedupeGroupID = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
