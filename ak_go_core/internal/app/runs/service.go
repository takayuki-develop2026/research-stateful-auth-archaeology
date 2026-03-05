package runs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

type ServiceConfig struct {
	// kept for backward compatibility with httpx/router.go
	DefaultPolicyVersion   string
	DefaultPipelineVersion string
}

type Service struct {
	db  *pgxpool.Pool
	cfg ServiceConfig
}

func NewService(db *pgxpool.Pool, cfg ServiceConfig) *Service {
	if strings.TrimSpace(cfg.DefaultPolicyVersion) == "" {
		cfg.DefaultPolicyVersion = "policy_v1_published"
	}
	if strings.TrimSpace(cfg.DefaultPipelineVersion) == "" {
		cfg.DefaultPipelineVersion = "v3"
	}
	return &Service{db: db, cfg: cfg}
}

type APIError struct {
	HTTPStatus int
	Type       string
	Message    string
}

type CreateRunInput struct {
	ProjectID       string
	PolicyVersion   string // now mapped to runs.policy_version_id (text)
	PipelineVersion string
	Mode            *int
	RequestKey      string // idempotency -> runs.run_key
	TraceID         string
	Source          string
}

/*
Compatibility contract (API):
- State: internal progress (queued/running/done/failed/created/...)
- Status: public notable status (failed only for now; review_required is not modeled in current runs table)
- Result: pending/success/failed
*/
type CreateRunOutput struct {
	RunID   string `json:"run_id"`
	TraceID string `json:"trace_id"`
	State   string `json:"state"`
	Status  string `json:"status,omitempty"`
	Result  string `json:"result"`
	Note    string `json:"note,omitempty"`
}

type Run struct {
	RunID           string `json:"run_id"`
	TraceID         string `json:"trace_id"`
	ProjectID       string `json:"project_id"`
	PolicyVersion   string `json:"policy_version"`
	PipelineVersion string `json:"pipeline_version"`

	State  string `json:"state"`
	Status string `json:"status,omitempty"`
	Result string `json:"result"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	RequestKey *string  `json:"request_key,omitempty"`
}

type RunEvent struct {
	EventSeq   int64           `json:"event_seq"`
	EventName  string          `json:"event_name"`
	TraceID    string          `json:"trace_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type RunEventsOut struct {
	RunID  string     `json:"run_id"`
	Events []RunEvent `json:"events"`
}

type RunArtifact struct {
	ArtifactKind string          `json:"artifact_kind"`
	ContentJSON  json.RawMessage `json:"content_json"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type RunArtifactsOut struct {
	RunID     string        `json:"run_id"`
	Artifacts []RunArtifact `json:"artifacts"`
}

// --------------------
// Helpers
// --------------------

func normalizeRunID(s string) string { return strings.TrimSpace(s) }
func normalizeTraceID(s string) string { return strings.TrimSpace(s) }

func normalizeRunState(dbStatus string) string {
	s := strings.TrimSpace(dbStatus)
	if s == "" {
		return "queued"
	}
	switch s {
	case "queued", "created", "running", "done", "failed":
		return s
	default:
		// keep as-is (API must not break)
		return s
	}
}

func derivePublicStatusAndResult(normalizedState string) (publicStatus string, result string) {
	st := strings.TrimSpace(normalizedState)
	switch st {
	case "done":
		return "", "success"
	case "failed":
		return "failed", "failed"
	case "queued", "created", "running", "":
		return "", "pending"
	default:
		ls := strings.ToLower(st)
		if strings.Contains(ls, "fail") || strings.Contains(ls, "error") {
			return "failed", "failed"
		}
		return "", "pending"
	}
}

func marshalJSONOrEmpty(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 5 * time.Second
	}
	return context.WithTimeout(ctx, d)
}

func isPgUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "23505"
	}
	return false
}

func newTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ulid.Make().String()
	}
	return hex.EncodeToString(b)
}

// --------------------
// Service methods
// --------------------

func (s *Service) CreateRun(ctx context.Context, in CreateRunInput) (CreateRunOutput, *APIError) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return CreateRunOutput{}, &APIError{HTTPStatus: 400, Type: "ValidationError", Message: "project_id is required"}
	}

	policyID := strings.TrimSpace(in.PolicyVersion)
	if policyID == "" {
		policyID = s.cfg.DefaultPolicyVersion
	}
	pipeline := strings.TrimSpace(in.PipelineVersion)
	if pipeline == "" {
		pipeline = s.cfg.DefaultPipelineVersion
	}

	// mode: store as text (nullable)
	var modeText *string
	if in.Mode != nil {
		m := fmt.Sprintf("Mode%d", *in.Mode)
		modeText = &m
	}

	requestKey := strings.TrimSpace(in.RequestKey)
	if len(requestKey) > 256 {
		requestKey = ""
	}
	var rk any = nil
	if requestKey != "" {
		rk = requestKey // mapped to runs.run_key
	}

	traceID := normalizeTraceID(in.TraceID)
	// DB trace_id is uuid; if caller passes non-uuid, insertion fails.
	// safest path: if empty, let DB default generate uuid.
	if traceID == "" {
		_ = newTraceID() // keep for client-side trace; not used in DB insert
	}

	// 1) idempotency reuse by run_key (requestKey)
	if requestKey != "" {
		existing, ok, apiErr := s.getRunByRunKey(ctx, projectID, requestKey)
		if apiErr != nil {
			return CreateRunOutput{}, apiErr
		}
		if ok {
			pubStatus, result := derivePublicStatusAndResult(existing.State)
			return CreateRunOutput{
				RunID:   existing.RunID,
				TraceID: existing.TraceID,
				State:   existing.State,
				Status:  pubStatus,
				Result:  result,
				Note:    "idempotent_reuse",
			}, nil
		}
	}

	tctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	tx, err := s.db.Begin(tctx)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "begin failed"}
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// 2) insert runs using DB truth:
	// runs(run_id uuid default, trace_id uuid default, status default queued (after migration),
	//      project_id, pipeline_version, policy_version_id, mode, run_key)
	var runID, dbTraceID, dbStatus string
	err = tx.QueryRow(tctx, `
		INSERT INTO runs(
			project_id,
			trace_id,
			pipeline_version,
			policy_version_id,
			mode,
			run_key,
			created_at,
			updated_at
		)
		VALUES (
			$1,
			CASE WHEN $2='' THEN gen_random_uuid() ELSE $2::uuid END,
			$3,
			$4,
			$5,
			$6,
			now(),
			now()
		)
		RETURNING run_id::text, trace_id::text, status::text
	`, projectID, traceID, pipeline, policyID, modeText, rk).Scan(&runID, &dbTraceID, &dbStatus)

	if err != nil {
		// run_key unique => reuse
		if requestKey != "" && isPgUniqueViolation(err) {
			existing, ok, apiErr := s.getRunByRunKey(ctx, projectID, requestKey)
			if ok && apiErr == nil {
				pubStatus, result := derivePublicStatusAndResult(existing.State)
				return CreateRunOutput{
					RunID:   existing.RunID,
					TraceID: existing.TraceID,
					State:   existing.State,
					Status:  pubStatus,
					Result:  result,
					Note:    "idempotent_reuse",
				}, nil
			}
		}
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "insert runs failed"}
	}

	// 3) insert run_events seq=1 (run.enqueued)
	payload := map[string]any{
		"project_id":        projectID,
		"policy_version_id": policyID,
		"pipeline_version":  pipeline,
		"mode":              in.Mode,
		"source":            strings.TrimSpace(in.Source),
	}
	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(tctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1::uuid, $2::uuid, 1, 'run.enqueued', $3::jsonb)
	`, runID, dbTraceID, pb)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "insert run_events failed"}
	}

	if err := tx.Commit(tctx); err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "commit failed"}
	}

	st := normalizeRunState(dbStatus)
	pubStatus, result := derivePublicStatusAndResult(st)

	return CreateRunOutput{
		RunID:   normalizeRunID(runID),
		TraceID: normalizeTraceID(dbTraceID),
		State:   st,
		Status:  pubStatus,
		Result:  result,
	}, nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (Run, bool, *APIError) {
	var out Run
	var dbStatus string
	var rk *string

	tctx, cancel := withTimeout(ctx, 5*time.Second)
	defer cancel()

	// DB truth columns (no policy_version/pipeline_version string names)
	err := s.db.QueryRow(tctx, `
		SELECT
			run_id::text,
			trace_id::text,
			project_id,
			policy_version_id,
			pipeline_version,
			status::text,
			created_at,
			updated_at,
			run_key
		FROM runs
		WHERE run_id = $1::uuid
	`, strings.TrimSpace(runID)).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID,
		&out.PolicyVersion, &out.PipelineVersion,
		&dbStatus, &out.CreatedAt, &out.UpdatedAt, &rk,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}

	out.RunID = normalizeRunID(out.RunID)
	out.TraceID = normalizeTraceID(out.TraceID)
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.PolicyVersion = strings.TrimSpace(out.PolicyVersion)
	out.PipelineVersion = strings.TrimSpace(out.PipelineVersion)

	out.RequestKey = rk
	out.State = normalizeRunState(dbStatus)
	out.Status, out.Result = derivePublicStatusAndResult(out.State)

	return out, true, nil
}

func (s *Service) GetRunEvents(ctx context.Context, runID string) (RunEventsOut, bool, *APIError) {
	_, ok, apiErr := s.GetRun(ctx, runID)
	if apiErr != nil {
		return RunEventsOut{}, false, apiErr
	}
	if !ok {
		return RunEventsOut{}, false, nil
	}

	tctx, cancel := withTimeout(ctx, 7*time.Second)
	defer cancel()

	rows, err := s.db.Query(tctx, `
		SELECT event_seq, event_name, trace_id::text, occurred_at, payload
		FROM run_events
		WHERE run_id = $1::uuid
		ORDER BY event_seq ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return RunEventsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}
	defer rows.Close()

	out := RunEventsOut{RunID: normalizeRunID(runID), Events: []RunEvent{}}
	for rows.Next() {
		var e RunEvent
		if err := rows.Scan(&e.EventSeq, &e.EventName, &e.TraceID, &e.OccurredAt, &e.Payload); err != nil {
			return RunEventsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "scan failed"}
		}
		e.TraceID = normalizeTraceID(e.TraceID)
		out.Events = append(out.Events, e)
	}
	if err := rows.Err(); err != nil {
		return RunEventsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "rows error"}
	}
	return out, true, nil
}

func (s *Service) GetRunArtifacts(ctx context.Context, runID string) (RunArtifactsOut, bool, *APIError) {
	_, ok, apiErr := s.GetRun(ctx, runID)
	if apiErr != nil {
		return RunArtifactsOut{}, false, apiErr
	}
	if !ok {
		return RunArtifactsOut{}, false, nil
	}

	tctx, cancel := withTimeout(ctx, 7*time.Second)
	defer cancel()

	rows, err := s.db.Query(tctx, `
		SELECT artifact_kind, content_json, created_at, updated_at
		FROM run_artifacts
		WHERE run_id = $1::uuid
		ORDER BY artifact_kind ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return RunArtifactsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}
	defer rows.Close()

	out := RunArtifactsOut{RunID: normalizeRunID(runID), Artifacts: []RunArtifact{}}
	for rows.Next() {
		var a RunArtifact
		if err := rows.Scan(&a.ArtifactKind, &a.ContentJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return RunArtifactsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "scan failed"}
		}
		out.Artifacts = append(out.Artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return RunArtifactsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "rows error"}
	}
	return out, true, nil
}

// GetTraceID returns runs.trace_id as text for a run_id.
func (s *Service) GetTraceID(ctx context.Context, runID string) (string, error) {
	var traceID string
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.db.QueryRow(tctx, `
		SELECT trace_id::text
		FROM runs
		WHERE run_id = $1::uuid
	`, strings.TrimSpace(runID)).Scan(&traceID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("run not found: %s", runID)
		}
		return "", fmt.Errorf("failed to query trace_id: %w", err)
	}
	return normalizeTraceID(traceID), nil
}

func (s *Service) getRunByRunKey(ctx context.Context, projectID, runKey string) (Run, bool, *APIError) {
	var out Run
	var dbStatus string
	var rk *string

	tctx, cancel := withTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.db.QueryRow(tctx, `
		SELECT
			run_id::text,
			trace_id::text,
			project_id,
			policy_version_id,
			pipeline_version,
			status::text,
			created_at,
			updated_at,
			run_key
		FROM runs
		WHERE project_id=$1 AND run_key=$2
		LIMIT 1
	`, projectID, runKey).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID,
		&out.PolicyVersion, &out.PipelineVersion,
		&dbStatus, &out.CreatedAt, &out.UpdatedAt, &rk,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "idempotency lookup failed"}
	}

	out.RunID = normalizeRunID(out.RunID)
	out.TraceID = normalizeTraceID(out.TraceID)
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.PolicyVersion = strings.TrimSpace(out.PolicyVersion)
	out.PipelineVersion = strings.TrimSpace(out.PipelineVersion)
	out.RequestKey = rk

	out.State = normalizeRunState(dbStatus)
	out.Status, out.Result = derivePublicStatusAndResult(out.State)

	return out, true, nil
}