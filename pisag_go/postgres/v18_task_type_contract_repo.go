package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type TaskTypeContractV18Repository struct{ db *sql.DB }

func NewTaskTypeContractV18Repository(db *sql.DB) *TaskTypeContractV18Repository {
	return &TaskTypeContractV18Repository{db: db}
}

func (r *TaskTypeContractV18Repository) Upsert(ctx context.Context, in run.TaskTypeContractUpsertInput) (run.TaskTypeContractChangeResult, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("project_id is required")
	}
	taskType := strings.TrimSpace(in.TaskType)
	if taskType == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("task_type is required")
	}
	pv := strings.TrimSpace(in.PipelineVersion)
	if pv == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("pipeline_version is required")
	}
	if strings.TrimSpace(in.InputContractEvidenceRef) == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("input_contract_evidence_ref is required")
	}
	if strings.TrimSpace(in.OutputContractEvidenceRef) == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("output_contract_evidence_ref is required")
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("trace_id is required")
	}
	createdByType := strings.TrimSpace(in.CreatedByType)
	if createdByType == "" {
		createdByType = "system"
	}

	var createdByID any = nil
	if in.CreatedByID != nil && strings.TrimSpace(*in.CreatedByID) != "" {
		createdByID = strings.TrimSpace(*in.CreatedByID)
	}

	var policyVID any = nil
	if in.PolicyVersionID != nil && strings.TrimSpace(*in.PolicyVersionID) != "" {
		policyVID = strings.TrimSpace(*in.PolicyVersionID)
	}

	var defaultMode any = nil
	if in.DefaultMode != nil && strings.TrimSpace(*in.DefaultMode) != "" {
		defaultMode = strings.TrimSpace(*in.DefaultMode)
	}

	var runID any = nil
	if in.RunID != nil && strings.TrimSpace(*in.RunID) != "" {
		runID = strings.TrimSpace(*in.RunID)
	}

	var idem any = nil
	if in.IdempotencyKey != nil && strings.TrimSpace(*in.IdempotencyKey) != "" {
		idem = strings.TrimSpace(*in.IdempotencyKey)
	}

	const q = `
SELECT contract_id, change_kind, found_existing
FROM public.task_type_contract_upsert_v18(
  $1::varchar,   -- project_id
  $2::varchar,   -- task_type
  $3::varchar,   -- pipeline_version
  $4::varchar,   -- policy_version_id (nullable)
  $5::boolean,   -- enabled
  $6::uuid,      -- input_contract_evidence_ref
  $7::uuid,      -- output_contract_evidence_ref
  $8::varchar,   -- default_mode (nullable)
  $9::varchar,   -- created_by_type
  $10::varchar,  -- created_by_id (nullable)
  $11::varchar,  -- trace_id
  $12::uuid,     -- run_id (nullable)
  $13::text      -- idempotency_key (nullable)
);
`

	var out run.TaskTypeContractChangeResult
	if err := r.db.QueryRowContext(
		ctx, q,
		projectID,
		taskType,
		pv,
		policyVID,
		in.Enabled,
		in.InputContractEvidenceRef,
		in.OutputContractEvidenceRef,
		defaultMode,
		createdByType,
		createdByID,
		traceID,
		runID,
		idem,
	).Scan(&out.ContractID, &out.ChangeKind, &out.FoundExisting); err != nil {
		return run.TaskTypeContractChangeResult{}, err
	}

	return out, nil
}

func (r *TaskTypeContractV18Repository) Enable(ctx context.Context, in run.TaskTypeContractToggleInput) (run.TaskTypeContractChangeResult, error) {
	return r.toggle(ctx, "public.task_type_contract_enable_v18", in)
}
func (r *TaskTypeContractV18Repository) Disable(ctx context.Context, in run.TaskTypeContractToggleInput) (run.TaskTypeContractChangeResult, error) {
	return r.toggle(ctx, "public.task_type_contract_disable_v18", in)
}

func (r *TaskTypeContractV18Repository) toggle(ctx context.Context, fn string, in run.TaskTypeContractToggleInput) (run.TaskTypeContractChangeResult, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("project_id is required")
	}
	taskType := strings.TrimSpace(in.TaskType)
	if taskType == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("task_type is required")
	}
	pv := strings.TrimSpace(in.PipelineVersion)
	if pv == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("pipeline_version is required")
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return run.TaskTypeContractChangeResult{}, errors.New("trace_id is required")
	}
	createdByType := strings.TrimSpace(in.CreatedByType)
	if createdByType == "" {
		createdByType = "system"
	}

	var createdByID any = nil
	if in.CreatedByID != nil && strings.TrimSpace(*in.CreatedByID) != "" {
		createdByID = strings.TrimSpace(*in.CreatedByID)
	}

	var runID any = nil
	if in.RunID != nil && strings.TrimSpace(*in.RunID) != "" {
		runID = strings.TrimSpace(*in.RunID)
	}

	var idem any = nil
	if in.IdempotencyKey != nil && strings.TrimSpace(*in.IdempotencyKey) != "" {
		idem = strings.TrimSpace(*in.IdempotencyKey)
	}

	q := `
SELECT contract_id, change_kind, found_existing
FROM ` + fn + `(
  $1::varchar,  -- project_id
  $2::varchar,  -- task_type
  $3::varchar,  -- pipeline_version
  $4::varchar,  -- trace_id
  $5::varchar,  -- created_by_type
  $6::varchar,  -- created_by_id (nullable)
  $7::uuid,     -- run_id (nullable)
  $8::text      -- idempotency_key (nullable)
);
`

	var out run.TaskTypeContractChangeResult
	if err := r.db.QueryRowContext(
		ctx, q,
		projectID,
		taskType,
		pv,
		traceID,
		createdByType,
		createdByID,
		runID,
		idem,
	).Scan(&out.ContractID, &out.ChangeKind, &out.FoundExisting); err != nil {
		return run.TaskTypeContractChangeResult{}, err
	}
	return out, nil
}
