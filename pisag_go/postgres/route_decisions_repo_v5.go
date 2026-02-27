package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type RouteDecisionsRepoV5 struct{ db *sql.DB }

func NewRouteDecisionsRepoV5(db *sql.DB) *RouteDecisionsRepoV5 { return &RouteDecisionsRepoV5{db: db} }

func (r *RouteDecisionsRepoV5) InsertIfAbsent(ctx context.Context, in run.RouteDecisionInsertInput) (run.RouteDecisionInsertResult, error) {
	// trim
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.SubjectType = strings.TrimSpace(in.SubjectType)
	in.SubjectInternalID = strings.TrimSpace(in.SubjectInternalID)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.RoutingVersion = strings.TrimSpace(in.RoutingVersion)
	in.InputFingerprint = strings.TrimSpace(in.InputFingerprint)
	in.WhyEvidenceRef = strings.TrimSpace(in.WhyEvidenceRef)
	in.UtlCommitEventKey = strings.TrimSpace(in.UtlCommitEventKey)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.RunID = strings.TrimSpace(in.RunID)

	if in.ProjectID == "" || in.SubjectType == "" || in.SubjectInternalID == "" {
		return run.RouteDecisionInsertResult{}, errors.New("project_id/subject_type/subject_internal_id are required")
	}
	if in.PolicyVersion == "" || in.PipelineVersion == "" || in.RoutingVersion == "" {
		return run.RouteDecisionInsertResult{}, errors.New("policy_version/pipeline_version/routing_version are required")
	}
	if len(in.InputFingerprint) != 64 {
		return run.RouteDecisionInsertResult{}, errors.New("input_fingerprint must be 64 hex")
	}
	if in.WhyEvidenceRef == "" || in.UtlCommitEventKey == "" || in.TraceID == "" || in.RunID == "" {
		return run.RouteDecisionInsertResult{}, errors.New("why_evidence_ref/utl_commit_event_key/trace_id/run_id are required")
	}

	const q = `
WITH existing AS (
  SELECT decision_id::text
  FROM public.route_decisions
  WHERE project_id=$1 AND subject_type=$2 AND subject_internal_id=$3 AND policy_version=$4
  LIMIT 1
),
ins AS (
  INSERT INTO public.route_decisions(
    project_id, subject_type, subject_internal_id,
    policy_version, pipeline_version, routing_version,
    input_fingerprint,
    chosen_route_id, chosen_provider_id,
    fallback_used,
    status, denied_reason,
    why, why_evidence_ref,
    utl_commit_event_key,
    trace_id, run_id
  )
  VALUES (
    $1,$2,$3,
    $4,$5,$6,
    $7,
    NULLIF($8::text,'')::uuid, NULLIF($9::text,'')::uuid,
    $10,
    $11, $12,
    $13::jsonb, $14::uuid,
    $15,
    $16, $17::uuid
  )
  ON CONFLICT (project_id, subject_type, subject_internal_id, policy_version) DO NOTHING
  RETURNING decision_id::text
)
SELECT
  COALESCE((SELECT decision_id FROM ins), (SELECT decision_id FROM existing)) AS decision_id,
  EXISTS(SELECT 1 FROM existing) AS found_existing;
`
	var out run.RouteDecisionInsertResult
	var chosenRoute, chosenProv string
	if in.ChosenRouteID != nil {
		chosenRoute = *in.ChosenRouteID
	}
	if in.ChosenProviderID != nil {
		chosenProv = *in.ChosenProviderID
	}

	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID, in.SubjectType, in.SubjectInternalID, in.PolicyVersion,
		in.PipelineVersion, in.RoutingVersion, in.InputFingerprint,
		chosenRoute, chosenProv,
		in.FallbackUsed,
		in.Status, in.DeniedReason,
		string(in.WhyJSON), in.WhyEvidenceRef,
		in.UtlCommitEventKey,
		in.TraceID, in.RunID,
	).Scan(&out.DecisionID, &out.FoundExisting); err != nil {
		return run.RouteDecisionInsertResult{}, err
	}
	return out, nil
}

func (r *RouteDecisionsRepoV5) GetByUnique(ctx context.Context, projectID, subjectType, subjectInternalID, policyVersion string) (run.RouteDecisionInsertInput, string, error) {
	projectID = strings.TrimSpace(projectID)
	subjectType = strings.TrimSpace(subjectType)
	subjectInternalID = strings.TrimSpace(subjectInternalID)
	policyVersion = strings.TrimSpace(policyVersion)
	if projectID == "" || subjectType == "" || subjectInternalID == "" || policyVersion == "" {
		return run.RouteDecisionInsertInput{}, "", errors.New("project_id/subject_type/subject_internal_id/policy_version are required")
	}

	const q = `
SELECT decision_id::text,
       project_id, subject_type, subject_internal_id,
       policy_version, pipeline_version, routing_version,
       input_fingerprint,
       chosen_route_id::text, chosen_provider_id::text,
       fallback_used,
       status, denied_reason,
       why::text, why_evidence_ref::text,
       utl_commit_event_key,
       trace_id, run_id::text
FROM public.route_decisions
WHERE project_id=$1 AND subject_type=$2 AND subject_internal_id=$3 AND policy_version=$4
LIMIT 1;
`
	var decisionID string
	var in run.RouteDecisionInsertInput
	var chosenRoute, chosenProv sql.NullString
	var denied sql.NullString
	var whyStr string
	if err := r.db.QueryRowContext(ctx, q, projectID, subjectType, subjectInternalID, policyVersion).Scan(
		&decisionID,
		&in.ProjectID, &in.SubjectType, &in.SubjectInternalID,
		&in.PolicyVersion, &in.PipelineVersion, &in.RoutingVersion,
		&in.InputFingerprint,
		&chosenRoute, &chosenProv,
		&in.FallbackUsed,
		&in.Status, &denied,
		&whyStr, &in.WhyEvidenceRef,
		&in.UtlCommitEventKey,
		&in.TraceID, &in.RunID,
	); err != nil {
		return run.RouteDecisionInsertInput{}, "", err
	}
	if chosenRoute.Valid {
		v := chosenRoute.String
		in.ChosenRouteID = &v
	}
	if chosenProv.Valid {
		v := chosenProv.String
		in.ChosenProviderID = &v
	}
	if denied.Valid {
		v := denied.String
		in.DeniedReason = &v
	}
	in.WhyJSON = []byte(whyStr)
	return in, decisionID, nil
}