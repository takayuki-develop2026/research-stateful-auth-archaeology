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

type DedupeRepository struct{ db *sql.DB }

func NewDedupeRepository(db *sql.DB) *DedupeRepository {
	return &DedupeRepository{db: db}
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ------------------------------------------------------------
// run.DedupeRepo
// ------------------------------------------------------------

func (r *DedupeRepository) UpsertGroup(
	ctx context.Context,
	in run.DedupeGroupUpsertInput,
) (run.DedupeGroupUpsertResult, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.CandidateType = strings.TrimSpace(in.CandidateType)
	in.DedupeKey = strings.TrimSpace(in.DedupeKey)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" || in.CandidateType == "" || len(in.DedupeKey) != 64 {
		return run.DedupeGroupUpsertResult{}, errors.New("project_id/run_id/trace_id/candidate_type/dedupe_key(64) are required")
	}

	const q = `
WITH existing AS (
  SELECT id FROM public.dedupe_groups
  WHERE project_id=$1 AND candidate_type=$2 AND dedupe_key=$3
  LIMIT 1
),
upsert AS (
  INSERT INTO public.dedupe_groups(
    project_id, candidate_type, dedupe_key, status,
    trace_id, run_id,
    created_at, updated_at
  )
  VALUES ($1,$2,$3,'open',$4,$5::uuid,now(),now())
  ON CONFLICT (project_id, candidate_type, dedupe_key) DO UPDATE
    SET trace_id=EXCLUDED.trace_id,
        run_id=EXCLUDED.run_id,
        updated_at=now()
  RETURNING id
)
SELECT (SELECT id FROM upsert) AS group_id,
       EXISTS(SELECT 1 FROM existing) AS found_existing;
`
	var out run.DedupeGroupUpsertResult
	if err := r.db.QueryRowContext(ctx, q, in.ProjectID, in.CandidateType, in.DedupeKey, in.TraceID, in.RunID).
		Scan(&out.GroupID, &out.FoundExisting); err != nil {
		return run.DedupeGroupUpsertResult{}, err
	}
	return out, nil
}

func (r *DedupeRepository) LinkMember(
	ctx context.Context,
	projectID string,
	groupID, candidateID int64,
	memberRole, traceID, runID string,
) error {
	projectID = strings.TrimSpace(projectID)
	memberRole = strings.TrimSpace(memberRole)
	traceID = strings.TrimSpace(traceID)
	runID = strings.TrimSpace(runID)

	if projectID == "" || groupID <= 0 || candidateID <= 0 || traceID == "" || runID == "" {
		return errors.New("project_id/group_id/candidate_id/trace_id/run_id are required")
	}
	if memberRole == "" {
		memberRole = "member"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1) insert member (idempotent)
	const ins = `
INSERT INTO public.dedupe_group_members(project_id, group_id, candidate_id, member_role, created_at)
VALUES ($1,$2,$3,$4,now())
ON CONFLICT (group_id, candidate_id) DO NOTHING;
`
	if _, err := tx.ExecContext(ctx, ins, projectID, groupID, candidateID, memberRole); err != nil {
		return err
	}

	// 2) attach candidate->group + key (read dedupe_key from group)
	const updCand = `
UPDATE public.discovery_candidates c
SET dedupe_group_id = g.id,
    dedupe_key = g.dedupe_key,
    updated_at = now()
FROM public.dedupe_groups g
WHERE g.project_id=$1 AND g.id=$2
  AND c.project_id=$1 AND c.id=$3;
`
	if _, err := tx.ExecContext(ctx, updCand, projectID, groupID, candidateID); err != nil {
		return err
	}

	// 3) if group has >=2 members => status review_required (never downgrade resolved)
	const markGroup = `
WITH cnt AS (
  SELECT count(*) AS n FROM public.dedupe_group_members WHERE group_id=$2
)
UPDATE public.dedupe_groups g
SET status = CASE
  WHEN g.status='resolved' THEN g.status
  WHEN (SELECT n FROM cnt) >= 2 THEN 'review_required'
  ELSE g.status
END,
trace_id=$3,
run_id=$4::uuid,
updated_at=now()
WHERE g.project_id=$1 AND g.id=$2;
`
	if _, err := tx.ExecContext(ctx, markGroup, projectID, groupID, traceID, runID); err != nil {
		return err
	}

	// 4) promote member candidates to review_required (only if currently proposed)
	const promoteCandidates = `
UPDATE public.discovery_candidates c
SET status='review_required',
    review_requested_at = COALESCE(c.review_requested_at, now()),
    updated_at=now()
WHERE c.project_id=$1
  AND c.id IN (
    SELECT m.candidate_id
    FROM public.dedupe_group_members m
    WHERE m.group_id=$2
  )
  AND c.status='proposed';
`
	if _, err := tx.ExecContext(ctx, promoteCandidates, projectID, groupID); err != nil {
		return err
	}

	// 5) decision ledger (small json only)
	payload, _ := json.Marshal(map[string]any{
		"group_id":     groupID,
		"candidate_id": candidateID,
		"member_role":  memberRole,
		"reason":       "member_linked",
	})
	const insDecision = `
INSERT INTO public.dedupe_decisions(
  project_id, group_id,
  decided_by_type, decided_by,
  decision_type, decision_payload, decision_evidence_ref,
  trace_id, run_id,
  decided_at, created_at
) VALUES (
  $1,$2,
  'system', NULL,
  'propose_group', $3::jsonb, NULL,
  $4,$5::uuid,
  now(), now()
);
`
	if _, err := tx.ExecContext(ctx, insDecision, projectID, groupID, string(payload), traceID, runID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DedupeRepository) MarkGroupReviewRequired(
	ctx context.Context,
	projectID string,
	groupID int64,
	traceID, runID string,
) error {
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || groupID <= 0 || traceID == "" || runID == "" {
		return errors.New("project_id/group_id/trace_id/run_id are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const q = `
UPDATE public.dedupe_groups
SET status='review_required', trace_id=$3, run_id=$4::uuid, updated_at=now()
WHERE project_id=$1 AND id=$2 AND status <> 'resolved';
`
	if _, err := tx.ExecContext(ctx, q, projectID, groupID, traceID, runID); err != nil {
		return err
	}

	// promote group members' candidates to review_required (only if proposed)
	const promote = `
UPDATE public.discovery_candidates c
SET status='review_required',
    review_requested_at = COALESCE(c.review_requested_at, now()),
    updated_at=now()
WHERE c.project_id=$1
  AND c.id IN (SELECT candidate_id FROM public.dedupe_group_members WHERE group_id=$2)
  AND c.status='proposed';
`
	if _, err := tx.ExecContext(ctx, promote, projectID, groupID); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"group_id": groupID,
		"reason":   "explicit_mark_review_required",
	})
	const insDecision = `
INSERT INTO public.dedupe_decisions(
  project_id, group_id,
  decided_by_type, decided_by,
  decision_type, decision_payload, decision_evidence_ref,
  trace_id, run_id,
  decided_at, created_at
) VALUES (
  $1,$2,
  'system', NULL,
  'propose_group', $3::jsonb, NULL,
  $4,$5::uuid,
  now(), now()
);
`
	if _, err := tx.ExecContext(ctx, insDecision, projectID, groupID, string(payload), traceID, runID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DedupeRepository) ResolveGroup(ctx context.Context, in run.DedupeGroupResolveInput) error {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.ResolutionType = strings.TrimSpace(in.ResolutionType)

	if in.ProjectID == "" || in.GroupID <= 0 || in.RunID == "" || in.TraceID == "" || in.ResolutionType == "" {
		return errors.New("project_id/group_id/run_id/trace_id/resolution_type are required")
	}
	if in.ResolutionType == "choose_winner" && in.WinnerCandidateID == nil {
		return errors.New("winner_candidate_id is required for choose_winner")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var winner any = nil
	if in.WinnerCandidateID != nil {
		winner = *in.WinnerCandidateID
	}

	// 1) mark group resolved
	const upd = `
UPDATE public.dedupe_groups
SET status='resolved',
    winner_candidate_id=$3,
    resolution_type=$4,
    resolution_note_evidence_ref=$5::uuid,
    trace_id=$6,
    run_id=$7::uuid,
    updated_at=now()
WHERE project_id=$1 AND id=$2;
`
	if _, err := tx.ExecContext(ctx, upd,
		in.ProjectID, in.GroupID,
		winner, in.ResolutionType, in.ResolutionNoteEvidenceRef,
		in.TraceID, in.RunID,
	); err != nil {
		return err
	}

	// 2) write decision ledger (human resolution)
	decisionType := "merge_fields"
	switch in.ResolutionType {
	case "choose_winner":
		decisionType = "confirm_winner"
	case "merge_fields":
		decisionType = "merge_fields"
	case "reject_all":
		decisionType = "reject_all"
	default:
		decisionType = "merge_fields"
	}

	payload, _ := json.Marshal(map[string]any{
		"group_id":            in.GroupID,
		"resolution_type":     in.ResolutionType,
		"winner_candidate_id": in.WinnerCandidateID,
	})
	const insDecision = `
INSERT INTO public.dedupe_decisions(
  project_id, group_id,
  decided_by_type, decided_by,
  decision_type, decision_payload, decision_evidence_ref,
  trace_id, run_id,
  decided_at, created_at
) VALUES (
  $1,$2,
  'human', NULL,
  $3, $4::jsonb, $5::uuid,
  $6,$7::uuid,
  now(), now()
);
`
	if _, err := tx.ExecContext(ctx, insDecision,
		in.ProjectID, in.GroupID,
		decisionType, string(payload), in.ResolutionNoteEvidenceRef,
		in.TraceID, in.RunID,
	); err != nil {
		return err
	}

	// 3) promote winner to approved (only if choose_winner and current status is proposed/review_required)
	if in.WinnerCandidateID != nil && in.ResolutionType == "choose_winner" {
		const promoteWinner = `
UPDATE public.discovery_candidates
SET status='approved',
    decided_at=COALESCE(decided_at, now()),
    updated_at=now()
WHERE project_id=$1 AND id=$2 AND status IN ('review_required','proposed');
`
		if _, err := tx.ExecContext(ctx, promoteWinner, in.ProjectID, *in.WinnerCandidateID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ------------------------------------------------------------
// Optional helper: deterministic idempotency key for v18 link calls
// (DB fn ignores it for now, but Go layer requires it non-empty)
// ------------------------------------------------------------

func DedupeLinkIdempotencyKey(projectID string, groupID, candidateID int64, kind string) string {
	projectID = strings.TrimSpace(projectID)
	kind = strings.TrimSpace(kind)
	raw := projectID + "|" + kind + "|" + strconv.FormatInt(groupID, 10) + "|" + strconv.FormatInt(candidateID, 10)
	return "v18:link:" + kind + ":" + sha256hex(raw)
}
