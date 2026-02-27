package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type DiscoveryCandidateRepository struct{ db *sql.DB }

func NewDiscoveryCandidateRepository(db *sql.DB) *DiscoveryCandidateRepository {
	return &DiscoveryCandidateRepository{db: db}
}

// Implements run.DiscoveryCandidateRepo
func (r *DiscoveryCandidateRepository) Upsert(
	ctx context.Context,
	in run.DiscoveryCandidateUpsertInput,
) (run.DiscoveryCandidateUpsertResult, error) {
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
    first_seen_at, last_seen_at, seen_count,
    created_at, updated_at
  )
  VALUES (
    $1,$4,
    $2,$3,
    COALESCE($5,'proposed'),
    COALESCE($6,'normal'),
    $7,
    $8::uuid, $9::uuid, $10::uuid,
    $11::uuid, $12, $13, $14,
    now(), now(), 1,
    now(), now()
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
        seen_count = public.discovery_candidates.seen_count + 1,
        updated_at = now(),
        review_requested_at = CASE
          WHEN COALESCE($5, public.discovery_candidates.status)='review_required'
               AND public.discovery_candidates.review_requested_at IS NULL
          THEN now()
          ELSE public.discovery_candidates.review_requested_at
        END
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

func (r *DiscoveryCandidateRepository) Get(
	ctx context.Context,
	projectID string,
	candidateID int64,
) (run.DiscoveryCandidate, error) {
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
	var conf sql.NullFloat64
	var payloadRef, normRef, diffRef sql.NullString
	var dedupeKey sql.NullString
	var dedupeGroup sql.NullInt64

	if err := r.db.QueryRowContext(ctx, q, projectID, candidateID).Scan(
		&c.ID, &c.ProjectID, &c.SourceID,
		&c.CandidateType, &c.CandidateKey, &status, &risk, &conf,
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

	if conf.Valid {
		v := conf.Float64
		c.Confidence = &v
	}
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

func (r *DiscoveryCandidateRepository) ListOps(
	ctx context.Context,
	projectID string,
	mode string,
	limit int,
) ([]run.DiscoveryCandidate, error) {
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
		var conf sql.NullFloat64
		var payloadRef, normRef, diffRef sql.NullString
		var dedupeKey sql.NullString
		var dedupeGroup sql.NullInt64

		if err := rows.Scan(
			&c.ID, &c.ProjectID, &c.SourceID,
			&c.CandidateType, &c.CandidateKey, &status, &risk, &conf,
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

		if conf.Valid {
			v := conf.Float64
			c.Confidence = &v
		}
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
