package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DiscoveryOpsRepo struct{ db *sql.DB }

func NewDiscoveryOpsRepo(db *sql.DB) *DiscoveryOpsRepo { return &DiscoveryOpsRepo{db: db} }

type CandidateRow struct {
	ID         int64     `json:"id"`
	ProjectID  string    `json:"project_id"`
	SourceID   int64     `json:"source_id"`
	Type       string    `json:"candidate_type"`
	Status     string    `json:"status"`
	RiskLevel  string    `json:"risk_level"`
	SeenCount  int64     `json:"seen_count"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type CandidateDetail struct {
	CandidateRow
	CandidateKey string `json:"candidate_key"`
	RunID        string `json:"run_id"`
	TraceID      string `json:"trace_id"`
	PipelineVer  string `json:"pipeline_version"`
	PolicyVer    string `json:"policy_version"`

	PayloadEvidenceRef    *string `json:"payload_evidence_ref"`
	NormalizedEvidenceRef *string `json:"normalized_evidence_ref"`
	DiffEvidenceRef       *string `json:"diff_evidence_ref"`

	StaleAt       *time.Time `json:"stale_at"`
	ArchivedAt    *time.Time `json:"archived_at"`
	ArchiveReason *string    `json:"archive_reason"`

	RetryAttempts int64      `json:"retry_attempts"`
	RetryNextAt   *time.Time `json:"retry_next_at"`
	RetryLastCode *string    `json:"retry_last_code"`

	ApplyAttempts int64      `json:"apply_attempts"`
	ApplyNextAt   *time.Time `json:"apply_next_at"`
	ApplyLastCode *string    `json:"apply_last_code"`
}

type LifecycleEvent struct {
	EventType string    `json:"event_type"`
	ActorType string    `json:"actor_type"`
	ActorID   *string   `json:"actor_id"`
	Message   *string   `json:"message"`
	TraceID   string    `json:"trace_id"`
	RunID     string    `json:"run_id"`
	CreatedAt time.Time `json:"created_at"`
	DetailRef *string   `json:"detail_evidence_ref"`
}

type CandidateEvent struct {
	EventType string    `json:"event_type"`
	ActorType string    `json:"actor_type"`
	ActorID   *string   `json:"actor_id"`
	TraceID   string    `json:"trace_id"`
	RunID     string    `json:"run_id"`
	CreatedAt time.Time `json:"created_at"`
	NoteRef   *string   `json:"note_evidence_ref"`
}

type ListParams struct {
	ProjectID string
	Mode      string // stale|retry|apply_retry|archived
	Type      string
	Status    string
	Q         string
	OnlyDue   bool
	Limit     int
}

func (r *DiscoveryOpsRepo) ListCandidates(ctx context.Context, p ListParams) ([]CandidateRow, error) {
	p.ProjectID = strings.TrimSpace(p.ProjectID)
	p.Mode = strings.TrimSpace(p.Mode)
	p.Type = strings.TrimSpace(p.Type)
	p.Status = strings.TrimSpace(p.Status)
	p.Q = strings.TrimSpace(p.Q)
	if p.ProjectID == "" || p.Mode == "" {
		return nil, errors.New("project_id and mode are required")
	}
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 100
	}

	where := []string{"project_id = $1"}
	args := []any{p.ProjectID}
	i := 2

	// mode
	switch p.Mode {
	case "stale":
		where = append(where, "stale_at IS NOT NULL", "archived_at IS NULL")
	case "retry":
		where = append(where, "retry_next_at IS NOT NULL", "archived_at IS NULL")
		if p.OnlyDue {
			where = append(where, "retry_next_at <= now()")
		}
	case "apply_retry":
		where = append(where, "apply_next_at IS NOT NULL", "archived_at IS NULL")
		if p.OnlyDue {
			where = append(where, "apply_next_at <= now()")
		}
	case "archived":
		where = append(where, "archived_at IS NOT NULL")
	default:
		return nil, errors.New("mode must be stale|retry|apply_retry|archived")
	}

	if p.Type != "" {
		where = append(where, fmt.Sprintf("candidate_type = $%d", i))
		args = append(args, p.Type)
		i++
	}
	if p.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", i))
		args = append(args, p.Status)
		i++
	}
	if p.Q != "" {
		// numeric id or fuzzy
		if id, err := strconv.ParseInt(p.Q, 10, 64); err == nil {
			where = append(where, fmt.Sprintf("id = $%d", i))
			args = append(args, id)
			i++
		} else {
			where = append(where, fmt.Sprintf("(candidate_key ILIKE $%d OR trace_id ILIKE $%d OR retry_last_code ILIKE $%d OR apply_last_code ILIKE $%d)", i, i, i, i))
			args = append(args, "%"+p.Q+"%")
			i++
		}
	}

	q := `
SELECT id, project_id, source_id, candidate_type, status, risk_level, seen_count, last_seen_at
FROM public.discovery_candidates
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY last_seen_at DESC
LIMIT ` + strconv.Itoa(p.Limit) + `;
`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CandidateRow, 0, 32)
	for rows.Next() {
		var c CandidateRow
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.SourceID, &c.Type, &c.Status, &c.RiskLevel, &c.SeenCount, &c.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *DiscoveryOpsRepo) GetCandidate(ctx context.Context, id int64) (CandidateDetail, error) {
	const q = `
SELECT id, project_id, source_id, candidate_type, status, risk_level, seen_count, last_seen_at,
       candidate_key, run_id::text, trace_id, pipeline_version, policy_version,
       payload_evidence_ref::text, normalized_evidence_ref::text, diff_evidence_ref::text,
       stale_at, archived_at, archive_reason,
       retry_attempts, retry_next_at, retry_last_code,
       apply_attempts, apply_next_at, apply_last_code
FROM public.discovery_candidates
WHERE id=$1
LIMIT 1;
`
	var d CandidateDetail
	var payload, norm, diff sql.NullString
	var stale, arch sql.NullTime
	var archReason sql.NullString
	var retryNext sql.NullTime
	var retryCode sql.NullString
	var applyNext sql.NullTime
	var applyCode sql.NullString

	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&d.ID, &d.ProjectID, &d.SourceID, &d.Type, &d.Status, &d.RiskLevel, &d.SeenCount, &d.LastSeenAt,
		&d.CandidateKey, &d.RunID, &d.TraceID, &d.PipelineVer, &d.PolicyVer,
		&payload, &norm, &diff,
		&stale, &arch, &archReason,
		&d.RetryAttempts, &retryNext, &retryCode,
		&d.ApplyAttempts, &applyNext, &applyCode,
	); err != nil {
		return CandidateDetail{}, err
	}

	if payload.Valid {
		v := payload.String
		d.PayloadEvidenceRef = &v
	}
	if norm.Valid {
		v := norm.String
		d.NormalizedEvidenceRef = &v
	}
	if diff.Valid {
		v := diff.String
		d.DiffEvidenceRef = &v
	}
	if stale.Valid {
		v := stale.Time
		d.StaleAt = &v
	}
	if arch.Valid {
		v := arch.Time
		d.ArchivedAt = &v
	}
	if archReason.Valid {
		v := archReason.String
		d.ArchiveReason = &v
	}
	if retryNext.Valid {
		v := retryNext.Time
		d.RetryNextAt = &v
	}
	if retryCode.Valid {
		v := retryCode.String
		d.RetryLastCode = &v
	}
	if applyNext.Valid {
		v := applyNext.Time
		d.ApplyNextAt = &v
	}
	if applyCode.Valid {
		v := applyCode.String
		d.ApplyLastCode = &v
	}

	return d, nil
}

func (r *DiscoveryOpsRepo) ListEvents(ctx context.Context, candidateID int64, limit int) (map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// lifecycle
	lq := `
SELECT event_type, actor_type, actor_id, message, detail_evidence_ref::text,
       trace_id, run_id::text, created_at
FROM public.discovery_candidate_lifecycle_events
WHERE candidate_id=$1
ORDER BY created_at DESC
LIMIT $2;
`
	lrows, err := r.db.QueryContext(ctx, lq, candidateID, limit)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()

	life := make([]LifecycleEvent, 0, 16)
	for lrows.Next() {
		var e LifecycleEvent
		var actor sql.NullString
		var msg sql.NullString
		var ref sql.NullString
		if err := lrows.Scan(&e.EventType, &e.ActorType, &actor, &msg, &ref, &e.TraceID, &e.RunID, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			v := actor.String
			e.ActorID = &v
		}
		if msg.Valid {
			v := msg.String
			e.Message = &v
		}
		if ref.Valid {
			v := ref.String
			e.DetailRef = &v
		}
		life = append(life, e)
	}

	// candidate events
	cq := `
SELECT event_type, actor_type, actor_id, note_evidence_ref::text, trace_id, run_id::text, created_at
FROM public.discovery_candidate_events
WHERE candidate_id=$1
ORDER BY created_at DESC
LIMIT $2;
`
	crows, err := r.db.QueryContext(ctx, cq, candidateID, limit)
	if err != nil {
		return nil, err
	}
	defer crows.Close()

	evs := make([]CandidateEvent, 0, 16)
	for crows.Next() {
		var e CandidateEvent
		var actor sql.NullString
		var note sql.NullString
		if err := crows.Scan(&e.EventType, &e.ActorType, &actor, &note, &e.TraceID, &e.RunID, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			v := actor.String
			e.ActorID = &v
		}
		if note.Valid {
			v := note.String
			e.NoteRef = &v
		}
		evs = append(evs, e)
	}

	return map[string]any{
		"lifecycle_events": life,
		"candidate_events": evs,
	}, nil
}

// ----- Actions (POST) -----

func (r *DiscoveryOpsRepo) RequeueReview(ctx context.Context, projectID string, candidateID int64, traceID, runID string, reason string) error {
	return r.simpleUpdate(ctx, projectID, candidateID, traceID, runID,
		`UPDATE public.discovery_candidates
         SET status='review_required', review_requested_at=now(), stale_at=NULL, updated_at=now()
         WHERE project_id=$1 AND id=$2;`,
		"review_requeued", "requeued: "+reason)
}

func (r *DiscoveryOpsRepo) RetryNow(ctx context.Context, projectID string, candidateID int64, traceID, runID string) error {
	return r.simpleUpdate(ctx, projectID, candidateID, traceID, runID,
		`UPDATE public.discovery_candidates
         SET status='needs_retry', retry_next_at=now(), updated_at=now()
         WHERE project_id=$1 AND id=$2;`,
		"retry_scheduled", "retry_now")
}

func (r *DiscoveryOpsRepo) ApplyRetryNow(ctx context.Context, projectID string, candidateID int64, traceID, runID string) error {
	return r.simpleUpdate(ctx, projectID, candidateID, traceID, runID,
		`UPDATE public.discovery_candidates
         SET apply_next_at=now(), updated_at=now()
         WHERE project_id=$1 AND id=$2;`,
		"apply_retry_scheduled", "apply_retry_now")
}

func (r *DiscoveryOpsRepo) Archive(ctx context.Context, projectID string, candidateID int64, traceID, runID string, reason string) error {
	return r.simpleUpdate(ctx, projectID, candidateID, traceID, runID,
		`UPDATE public.discovery_candidates
         SET archived_at=now(), archive_reason=$3, updated_at=now()
         WHERE project_id=$1 AND id=$2;`,
		"archived", "archive: "+reason, reason)
}

func (r *DiscoveryOpsRepo) Unarchive(ctx context.Context, projectID string, candidateID int64, traceID, runID string, reason string) error {
	return r.simpleUpdate(ctx, projectID, candidateID, traceID, runID,
		`UPDATE public.discovery_candidates
         SET archived_at=NULL, archive_reason=NULL, status='review_required', review_requested_at=now(), updated_at=now()
         WHERE project_id=$1 AND id=$2;`,
		"unarchived", "unarchive: "+reason)
}

func (r *DiscoveryOpsRepo) simpleUpdate(ctx context.Context, projectID string, candidateID int64, traceID, runID string, updSQL string, eventType string, msg string, extraArgs ...any) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || candidateID <= 0 || strings.TrimSpace(traceID) == "" || strings.TrimSpace(runID) == "" {
		return errors.New("project_id/candidate_id/trace_id/run_id are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	args := []any{projectID, candidateID}
	args = append(args, extraArgs...)

	res, err := tx.ExecContext(ctx, updSQL, args...)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return errors.New("candidate not found or not in project")
	}

	const insEvt = `
INSERT INTO public.discovery_candidate_lifecycle_events(
  project_id, candidate_id, event_type, actor_type, actor_id,
  message, detail_evidence_ref, trace_id, run_id, created_at
) VALUES ($1,$2,$3,'system',NULL,$4,NULL,$5,$6::uuid,now());
`
	if _, err := tx.ExecContext(ctx, insEvt, projectID, candidateID, eventType, msg, traceID, runID); err != nil {
		return err
	}

	return tx.Commit()
}
