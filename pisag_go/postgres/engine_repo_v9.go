package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EngineRepoV9 struct{ db *sql.DB }

func NewEngineRepoV9(db *sql.DB) *EngineRepoV9 { return &EngineRepoV9{db: db} }

// -----------------------------
// Models (DB row shapes)
// -----------------------------

type EngineRunV9 struct {
	EngineRunID string // uuid
	ProjectID   string
	RunID       string // uuid
	TraceID     string // uuid

	TaskType        string
	Mode            string
	PipelineVersion string
	PolicyVersion   string

	PrincipalHash string // 64 hex
	InputHash     string // 64 hex
	Status        string

	DecisionID     *string
	IdempotencyKey string
	CacheKey       *string

	StartedAt  *time.Time
	FinishedAt *time.Time

	ErrorType            *string
	ErrorSummary         *string
	ErrorEvidenceAssetID *int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

type DecisionLedgerV9 struct {
	DecisionID  string // uuid
	ProjectID   string
	EngineRunID string // uuid

	DecisionType    string // route|plan|proposal|reject|review_required
	ResultJSON      []byte // json bytes
	RationaleJSON   []byte // json bytes
	ConstraintsJSON []byte // json bytes

	DecisionEvidenceAssetID int64

	CreatedByType string // system|user|service
	CreatedByID   *string

	PolicyVersion string
	CreatedAt     time.Time
}

type EngineCacheV9 struct {
	ID          int64
	ProjectID   string
	CacheKey    string // 64 hex
	EngineRunID string
	DecisionID  string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// -----------------------------
// Helpers
// -----------------------------

func v90Sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func must64hex(v string, name string) error {
	v = strings.TrimSpace(v)
	if len(v) != 64 {
		return fmt.Errorf("%s must be 64-hex", name)
	}
	return nil
}

// -----------------------------
// Evidence helpers (v18)
// -----------------------------

// Resolve evidence_assets.id by (project_id, evidence_ref uuid string)
func (r *EngineRepoV9) ResolveEvidenceAssetIDByRef(ctx context.Context, projectID, evidenceRef string) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	evidenceRef = strings.TrimSpace(evidenceRef)
	if projectID == "" || evidenceRef == "" {
		return 0, errors.New("project_id and evidence_ref are required")
	}

	const q = `
SELECT id
FROM public.evidence_assets
WHERE project_id=$1 AND evidence_ref=$2::uuid
LIMIT 1;
`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, projectID, evidenceRef).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// -----------------------------
// Engine Runs
// -----------------------------

type EngineRunUpsertStableInput struct {
	ProjectID string
	RunID     string // uuid text
	TraceID   string // uuid text

	TaskType        string
	Mode            string
	PipelineVersion string
	PolicyVersion   string

	PrincipalHash string // 64 hex
	InputHash     string // 64 hex

	IdempotencyKey string // scope included (v13 style)
}

type EngineRunUpsertStableResult struct {
	EngineRunID   string
	FoundExisting bool
	Status        string
}

func (r *EngineRepoV9) UpsertStableEngineRun(ctx context.Context, in EngineRunUpsertStableInput) (EngineRunUpsertStableResult, error) {
	// trim
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.TaskType = strings.TrimSpace(in.TaskType)
	in.Mode = strings.TrimSpace(in.Mode)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.PrincipalHash = strings.TrimSpace(in.PrincipalHash)
	in.InputHash = strings.TrimSpace(in.InputHash)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" {
		return EngineRunUpsertStableResult{}, errors.New("project_id/run_id/trace_id are required")
	}
	if in.TaskType == "" || in.Mode == "" || in.PipelineVersion == "" || in.PolicyVersion == "" {
		return EngineRunUpsertStableResult{}, errors.New("task_type/mode/pipeline_version/policy_version are required")
	}
	if err := must64hex(in.PrincipalHash, "principal_hash"); err != nil {
		return EngineRunUpsertStableResult{}, err
	}
	if err := must64hex(in.InputHash, "input_hash"); err != nil {
		return EngineRunUpsertStableResult{}, err
	}
	if in.IdempotencyKey == "" {
		return EngineRunUpsertStableResult{}, errors.New("idempotency_key is required")
	}

	// Uses UNIQUE(project_id, task_type, mode, pipeline_version, policy_version, principal_hash, input_hash)
	// Important: second call returns existing row; we still keep new run_id/trace_id in request context,
	// but SoT row remains unique to stable key.
	const q = `
WITH existing AS (
  SELECT engine_run_id::text, status
  FROM public.engine_runs_v9
  WHERE project_id=$1
    AND task_type=$2
    AND mode=$3
    AND pipeline_version=$4
    AND policy_version=$5
    AND principal_hash=$6
    AND input_hash=$7
  LIMIT 1
),
ins AS (
  INSERT INTO public.engine_runs_v9(
    project_id, run_id, trace_id,
    task_type, mode, pipeline_version, policy_version,
    principal_hash, input_hash,
    status, idempotency_key, created_at, updated_at
  )
  VALUES (
    $1,
    $8::uuid,
    $9::uuid,
    $2,$3,$4,$5,
    $6,$7,
    'queued', $10, now(), now()
  )
  ON CONFLICT (project_id, task_type, mode, pipeline_version, policy_version, principal_hash, input_hash)
  DO NOTHING
  RETURNING engine_run_id::text, status
)
SELECT
  COALESCE((SELECT engine_run_id FROM ins), (SELECT engine_run_id FROM existing)) AS engine_run_id,
  EXISTS(SELECT 1 FROM existing) AS found_existing,
  COALESCE((SELECT status FROM ins), (SELECT status FROM existing)) AS status;
`
	var out EngineRunUpsertStableResult
	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID,
		in.TaskType,
		in.Mode,
		in.PipelineVersion,
		in.PolicyVersion,
		in.PrincipalHash,
		in.InputHash,
		in.RunID,
		in.TraceID,
		in.IdempotencyKey,
	).Scan(&out.EngineRunID, &out.FoundExisting, &out.Status); err != nil {
		return EngineRunUpsertStableResult{}, err
	}
	return out, nil
}

func (r *EngineRepoV9) MarkEngineRunRunning(ctx context.Context, projectID, engineRunID string, cacheKey *string) error {
	projectID = strings.TrimSpace(projectID)
	engineRunID = strings.TrimSpace(engineRunID)
	if projectID == "" || engineRunID == "" {
		return errors.New("project_id and engine_run_id are required")
	}

	const q = `
UPDATE public.engine_runs_v9
SET status='running',
    started_at=COALESCE(started_at, now()),
    cache_key=COALESCE(cache_key, $3::char(64)),
    updated_at=now()
WHERE project_id=$1 AND engine_run_id=$2::uuid;
`
	var ck any = nil
	if cacheKey != nil && strings.TrimSpace(*cacheKey) != "" {
		v := strings.TrimSpace(*cacheKey)
		ck = v
	}
	_, err := r.db.ExecContext(ctx, q, projectID, engineRunID, ck)
	return err
}

func (r *EngineRepoV9) CompleteEngineRun(ctx context.Context, projectID, engineRunID string, status string, decisionID *string, cacheKey *string, errorType *string, errorSummary *string, errorEvidenceAssetID *int64) error {
	projectID = strings.TrimSpace(projectID)
	engineRunID = strings.TrimSpace(engineRunID)
	status = strings.TrimSpace(status)

	if projectID == "" || engineRunID == "" || status == "" {
		return errors.New("project_id/engine_run_id/status are required")
	}

	const q = `
UPDATE public.engine_runs_v9
SET status=$3::varchar,
    decision_id=NULLIF($4::text,'')::uuid,
    cache_key=NULLIF($5::text,'')::char(64),
    finished_at=now(),
    error_type=NULLIF($6::text,'')::varchar(64),
    error_summary=NULLIF($7::text,'')::varchar(256),
    error_evidence_asset_id=NULLIF($8::bigint,0),
    updated_at=now()
WHERE project_id=$1 AND engine_run_id=$2::uuid;
`
	var did string
	if decisionID != nil {
		did = strings.TrimSpace(*decisionID)
	}
	var ck string
	if cacheKey != nil {
		ck = strings.TrimSpace(*cacheKey)
	}
	var et string
	if errorType != nil {
		et = strings.TrimSpace(*errorType)
	}
	var es string
	if errorSummary != nil {
		es = strings.TrimSpace(*errorSummary)
	}
	var ee any = 0
	if errorEvidenceAssetID != nil && *errorEvidenceAssetID > 0 {
		ee = *errorEvidenceAssetID
	}

	_, err := r.db.ExecContext(ctx, q, projectID, engineRunID, status, did, ck, et, es, ee)
	return err
}

// -----------------------------
// Decision Ledger
// -----------------------------

type DecisionInsertInputV9 struct {
	ProjectID   string
	EngineRunID string

	DecisionType string

	ResultJSON      []byte
	RationaleJSON   []byte
	ConstraintsJSON []byte

	DecisionEvidenceAssetID int64

	CreatedByType string
	CreatedByID   *string

	PolicyVersion string
}

type DecisionInsertResultV9 struct {
	DecisionID string
}

func (r *EngineRepoV9) InsertDecision(ctx context.Context, in DecisionInsertInputV9) (DecisionInsertResultV9, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.EngineRunID = strings.TrimSpace(in.EngineRunID)
	in.DecisionType = strings.TrimSpace(in.DecisionType)
	in.CreatedByType = strings.TrimSpace(in.CreatedByType)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)

	if in.ProjectID == "" || in.EngineRunID == "" || in.DecisionType == "" {
		return DecisionInsertResultV9{}, errors.New("project_id/engine_run_id/decision_type are required")
	}
	if in.DecisionEvidenceAssetID <= 0 {
		return DecisionInsertResultV9{}, errors.New("decision_evidence_asset_id is required")
	}
	if in.CreatedByType == "" || in.PolicyVersion == "" {
		return DecisionInsertResultV9{}, errors.New("created_by_type and policy_version are required")
	}
	if len(in.ResultJSON) == 0 {
		in.ResultJSON = []byte(`{}`)
	}
	if len(in.RationaleJSON) == 0 {
		in.RationaleJSON = []byte(`{}`)
	}
	if len(in.ConstraintsJSON) == 0 {
		in.ConstraintsJSON = []byte(`{}`)
	}

	const q = `
INSERT INTO public.decision_ledger_v9(
  project_id, engine_run_id, decision_type,
  result_json, rationale_json, constraints_json,
  decision_evidence_asset_id,
  created_by_type, created_by_id,
  policy_version,
  created_at
)
VALUES (
  $1,
  $2::uuid,
  $3,
  $4::jsonb,
  $5::jsonb,
  $6::jsonb,
  $7::bigint,
  $8,
  NULLIF($9::text,'')::varchar(128),
  $10,
  now()
)
RETURNING decision_id::text;
`
	var out DecisionInsertResultV9
	var createdBy string
	if in.CreatedByID != nil {
		createdBy = strings.TrimSpace(*in.CreatedByID)
	}
	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID,
		in.EngineRunID,
		in.DecisionType,
		string(in.ResultJSON),
		string(in.RationaleJSON),
		string(in.ConstraintsJSON),
		in.DecisionEvidenceAssetID,
		in.CreatedByType,
		createdBy,
		in.PolicyVersion,
	).Scan(&out.DecisionID); err != nil {
		return DecisionInsertResultV9{}, err
	}
	return out, nil
}

type DecisionGetResultV9 struct {
	DecisionID              string
	DecisionType            string
	ResultJSON              []byte
	RationaleJSON           []byte
	ConstraintsJSON         []byte
	DecisionEvidenceAssetID int64
	PolicyVersion           string
	CreatedAt               time.Time
}

func (r *EngineRepoV9) GetDecision(ctx context.Context, decisionID string) (DecisionGetResultV9, error) {
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return DecisionGetResultV9{}, errors.New("decision_id is required")
	}

	const q = `
SELECT decision_id::text, decision_type,
       result_json::text, rationale_json::text, constraints_json::text,
       decision_evidence_asset_id,
       policy_version,
       created_at
FROM public.decision_ledger_v9
WHERE decision_id=$1::uuid
LIMIT 1;
`
	var out DecisionGetResultV9
	var rj, wj, cj string
	if err := r.db.QueryRowContext(ctx, q, decisionID).Scan(
		&out.DecisionID, &out.DecisionType,
		&rj, &wj, &cj,
		&out.DecisionEvidenceAssetID,
		&out.PolicyVersion,
		&out.CreatedAt,
	); err != nil {
		return DecisionGetResultV9{}, err
	}
	out.ResultJSON = []byte(rj)
	out.RationaleJSON = []byte(wj)
	out.ConstraintsJSON = []byte(cj)
	return out, nil
}

// -----------------------------
// Cache
// -----------------------------

func (r *EngineRepoV9) GetCache(ctx context.Context, projectID, cacheKey string) (*EngineCacheV9, error) {
	projectID = strings.TrimSpace(projectID)
	cacheKey = strings.TrimSpace(cacheKey)
	if projectID == "" || cacheKey == "" {
		return nil, errors.New("project_id and cache_key are required")
	}
	if err := must64hex(cacheKey, "cache_key"); err != nil {
		return nil, err
	}

	const q = `
SELECT id, project_id, cache_key, engine_run_id::text, decision_id::text, expires_at, created_at
FROM public.engine_cache_v9
WHERE project_id=$1 AND cache_key=$2
  AND expires_at > now()
LIMIT 1;
`
	var c EngineCacheV9
	err := r.db.QueryRowContext(ctx, q, projectID, cacheKey).Scan(
		&c.ID, &c.ProjectID, &c.CacheKey, &c.EngineRunID, &c.DecisionID, &c.ExpiresAt, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *EngineRepoV9) UpsertCache(ctx context.Context, projectID, cacheKey, engineRunID, decisionID string, expiresAt time.Time) error {
	projectID = strings.TrimSpace(projectID)
	cacheKey = strings.TrimSpace(cacheKey)
	engineRunID = strings.TrimSpace(engineRunID)
	decisionID = strings.TrimSpace(decisionID)

	if projectID == "" || cacheKey == "" || engineRunID == "" || decisionID == "" {
		return errors.New("project_id/cache_key/engine_run_id/decision_id are required")
	}
	if err := must64hex(cacheKey, "cache_key"); err != nil {
		return err
	}

	const q = `
INSERT INTO public.engine_cache_v9(project_id, cache_key, engine_run_id, decision_id, expires_at, created_at)
VALUES ($1, $2::char(64), $3::uuid, $4::uuid, $5, now())
ON CONFLICT (project_id, cache_key)
DO UPDATE SET engine_run_id=EXCLUDED.engine_run_id,
              decision_id=EXCLUDED.decision_id,
              expires_at=EXCLUDED.expires_at;
`
	_, err := r.db.ExecContext(ctx, q, projectID, cacheKey, engineRunID, decisionID, expiresAt)
	return err
}

// Compute cache_key per v9 contract (sha256 hex)
func ComputeEngineCacheKeyV9(projectID, taskType, mode, pipelineVersion, policyVersion, principalHash, inputHash string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	taskType = strings.TrimSpace(taskType)
	mode = strings.TrimSpace(mode)
	pipelineVersion = strings.TrimSpace(pipelineVersion)
	policyVersion = strings.TrimSpace(policyVersion)
	principalHash = strings.TrimSpace(principalHash)
	inputHash = strings.TrimSpace(inputHash)

	if projectID == "" || taskType == "" || mode == "" || pipelineVersion == "" || policyVersion == "" {
		return "", errors.New("project_id/task_type/mode/pipeline_version/policy_version are required")
	}
	if err := must64hex(principalHash, "principal_hash"); err != nil {
		return "", err
	}
	if err := must64hex(inputHash, "input_hash"); err != nil {
		return "", err
	}

	s := strings.Join([]string{
		projectID,
		taskType,
		mode,
		pipelineVersion,
		policyVersion,
		principalHash,
		inputHash,
	}, "|")
	return v90Sha256Hex(s), nil
}
