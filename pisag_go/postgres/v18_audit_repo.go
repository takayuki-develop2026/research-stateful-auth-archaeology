package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type AuditV18Repository struct{ db *sql.DB }

func NewAuditV18Repository(db *sql.DB) *AuditV18Repository { return &AuditV18Repository{db: db} }

func (r *AuditV18Repository) Append(ctx context.Context, in run.AuditAppendInput) (run.AuditAppendResult, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.AuditAppendResult{}, errors.New("project_id is required")
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return run.AuditAppendResult{}, errors.New("trace_id is required")
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		return run.AuditAppendResult{}, errors.New("action is required")
	}
	actorType := strings.TrimSpace(in.ActorType)
	if actorType == "" {
		return run.AuditAppendResult{}, errors.New("actor_type is required")
	}
	result := strings.TrimSpace(in.Result)
	if result == "" {
		return run.AuditAppendResult{}, errors.New("result is required")
	}

	var runID any = nil
	if in.RunID != nil && strings.TrimSpace(*in.RunID) != "" {
		runID = strings.TrimSpace(*in.RunID)
	}
	var actorID any = nil
	if in.ActorID != nil && strings.TrimSpace(*in.ActorID) != "" {
		actorID = strings.TrimSpace(*in.ActorID)
	}
	var targetType any = nil
	if in.TargetType != nil && strings.TrimSpace(*in.TargetType) != "" {
		targetType = strings.TrimSpace(*in.TargetType)
	}
	var targetID any = nil
	if in.TargetID != nil && strings.TrimSpace(*in.TargetID) != "" {
		targetID = strings.TrimSpace(*in.TargetID)
	}
	var reason any = nil
	if in.Reason != nil && strings.TrimSpace(*in.Reason) != "" {
		reason = strings.TrimSpace(*in.Reason)
	}
	var meta any = nil
	if len(in.MetaJSON) > 0 {
		meta = string(in.MetaJSON) // cast to jsonb in SQL
	}
	var idem any = nil
	if in.IdempotencyKey != nil && strings.TrimSpace(*in.IdempotencyKey) != "" {
		idem = strings.TrimSpace(*in.IdempotencyKey)
	}

	const q = `
SELECT audit_event_id, found_existing
FROM public.audit_event_append_v18(
  $1::varchar,   -- project_id
  $2::varchar,   -- trace_id
  $3::varchar,   -- run_id (nullable)
  $4::varchar,   -- action
  $5::varchar,   -- actor_type
  $6::varchar,   -- actor_id (nullable)
  $7::varchar,   -- target_type (nullable)
  $8::varchar,   -- target_id (nullable)
  $9::varchar,   -- result
  $10::text,     -- reason (nullable)
  $11::jsonb,    -- meta (nullable)
  $12::text      -- idempotency_key (nullable)
);
`

	var out run.AuditAppendResult
	if err := r.db.QueryRowContext(ctx, q,
		projectID, traceID, runID, action,
		actorType, actorID, targetType, targetID,
		result, reason, meta, idem,
	).Scan(&out.AuditEventID, &out.FoundExisting); err != nil {
		return run.AuditAppendResult{}, err
	}
	return out, nil
}
