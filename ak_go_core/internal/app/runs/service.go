package runs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

type ServiceConfig struct {
	DefaultPolicyVersion   string
	DefaultPipelineVersion string
}

type Service struct {
	db  *pgxpool.Pool
	cfg ServiceConfig
}

func NewService(db *pgxpool.Pool, cfg ServiceConfig) *Service {
	// safety defaults
	if strings.TrimSpace(cfg.DefaultPolicyVersion) == "" {
		cfg.DefaultPolicyVersion = "v3"
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
	PolicyVersion   string
	PipelineVersion string
	Mode            *int
	RequestKey      string
	TraceID         string
	Source          string
}

/*
P0 contract (API):
- state: internal progress (queued/running/done/failed/review_required/blocked...)
- status: public 2-value status (review_required/failed) OR omitted when not applicable
- result: pending/success/failed
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

	// P0: normalized output
	State  string `json:"state"`
	Status string `json:"status,omitempty"`
	Result string `json:"result"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	RequestKey *string   `json:"request_key,omitempty"`
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
// P0 normalization helpers
// --------------------

func normalizeRunID(s string) string {
	// runs.run_id is CHAR(26) in some designs; or ULID(26) string. Trim spaces defensively.
	return strings.TrimSpace(s)
}

func normalizeTraceID(s string) string {
	return strings.TrimSpace(s)
}

// normalizeRunState normalizes DB "state" into known buckets but does not destroy unknown values.
func normalizeRunState(dbState string) string {
	s := strings.TrimSpace(dbState)
	if s == "" {
		return "queued"
	}

	// treat "run.blocked.*" and "blocked*" as state=blocked
	if strings.HasPrefix(s, "run.blocked.") || strings.HasPrefix(s, "blocked") {
		return "blocked"
	}

	switch s {
	case "queued", "running", "done", "failed", "review_required":
		return s
	}

	// fallback: keep as-is (API must not break)
	return s
}

// derivePublicStatusAndResult derives API status/result from normalized state.
// - status: only for public notable terminal-ish states (review_required/failed)
// - result: pending/success/failed (public simple)
func derivePublicStatusAndResult(normalizedState string) (publicStatus string, result string) {
	st := strings.TrimSpace(normalizedState)

	// 1) blocked => public status omitted / result pending
	if strings.HasPrefix(st, "run.blocked.") || strings.HasPrefix(st, "blocked") {
		return "", "pending"
	}

	switch st {
	case "done":
		return "", "success"
	case "failed":
		return "failed", "failed"
	case "review_required":
		// P0 contract: review_required is NOT success; it is "pending human decision"
		return "review_required", "pending"
	case "queued", "running", "":
		return "", "pending"
	default:
		// conservative fallback
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

// --------------------
// Service methods
// --------------------

func (s *Service) CreateRun(ctx context.Context, in CreateRunInput) (CreateRunOutput, *APIError) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return CreateRunOutput{}, &APIError{HTTPStatus: 400, Type: "ValidationError", Message: "project_id is required"}
	}

	policy := strings.TrimSpace(in.PolicyVersion)
	if policy == "" {
		policy = s.cfg.DefaultPolicyVersion
	}

	pipeline := strings.TrimSpace(in.PipelineVersion)
	if pipeline == "" {
		pipeline = s.cfg.DefaultPipelineVersion
	}

	mode := 0
	if in.Mode != nil {
		mode = *in.Mode
	}

	requestKey := strings.TrimSpace(in.RequestKey)
	if len(requestKey) > 256 {
		requestKey = ""
	}

	traceID := normalizeTraceID(in.TraceID)
	if traceID == "" {
		traceID = newTraceID()
	}

	// ---------------------------------------------------------
	// 1) 冪等性チェック (Idempotency pre-check)
	// ---------------------------------------------------------
	if requestKey != "" {
		existing, ok, apiErr := s.getRunByRequestKey(ctx, requestKey)
		if apiErr != nil {
			return CreateRunOutput{}, apiErr
		}
		if ok {
			// existing.State is already normalized inside getRunByRequestKey
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

	// ---------------------------------------------------------
	// 2) トランザクション: runs挿入 + event挿入 + next_event_seq更新
	// ---------------------------------------------------------
	tctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	tx, err := s.db.Begin(tctx)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "begin failed"}
	}

	// P0: rollback should not depend on canceled context
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// Workerが期待する ULID (26文字) を生成
	runID := ulid.Make().String()

	var rk any = nil
	if requestKey != "" {
		rk = requestKey
	}

	// 2-a) runs テーブルに基本情報を挿入 (state='queued' で開始)
	_, err = tx.Exec(tctx, `
		INSERT INTO runs(
			run_id, trace_id, project_id, policy_version, pipeline_version,
			state, request_key
		)
		VALUES ($1, $2, $3, $4, $5, 'queued', $6)
	`, runID, traceID, projectID, policy, pipeline, rk)

	if err != nil {
		// request_key由来のunique違反のみを “冪等再利用” として扱う
		if requestKey != "" && isPgUniqueViolationOnRequestKey(err) {
			existing, ok, apiErr := s.getRunByRequestKey(ctx, requestKey)
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

	// 2-b) run_events に最初のイベント 'run.enqueued' (seq=1) を挿入
	payload := map[string]any{
		"project_id":       projectID,
		"policy_version":   policy,
		"pipeline_version": pipeline,
		"mode":             mode,
		"source":           strings.TrimSpace(in.Source),
	}
	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(tctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1, $2, 1, 'run.enqueued', $3::jsonb)
	`, runID, traceID, pb)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "insert run_events failed"}
	}

	// 2-c) ★最重要: runs テーブルの next_event_seq を 2 に進める
	_, err = tx.Exec(tctx, `
		UPDATE runs
		SET next_event_seq = 2,
			updated_at = now()
		WHERE run_id = $1
	`, runID)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "update sequence failed"}
	}

	// 2-d) commit
	if err := tx.Commit(tctx); err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "commit failed"}
	}

	// ---------------------------------------------------------
	// 3) 初期状態の返却
	// ---------------------------------------------------------
	st := normalizeRunState("queued")
	pubStatus, result := derivePublicStatusAndResult(st)

	return CreateRunOutput{
		RunID:   runID,
		TraceID: traceID,
		State:   st,
		Status:  pubStatus,
		Result:  result,
	}, nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (Run, bool, *APIError) {
	var out Run
	var requestKey *string
	var dbState string

	tctx, cancel := withTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.db.QueryRow(tctx, `
		SELECT
			run_id, trace_id, project_id, policy_version, pipeline_version,
			state, created_at, updated_at, request_key
		FROM runs
		WHERE run_id = $1
	`, strings.TrimSpace(runID)).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID, &out.PolicyVersion, &out.PipelineVersion,
		&dbState, &out.CreatedAt, &out.UpdatedAt, &requestKey,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}

	out.RunID = normalizeRunID(out.RunID)
	out.TraceID = normalizeTraceID(out.TraceID)
	out.RequestKey = requestKey

	out.State = normalizeRunState(dbState)
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
		SELECT event_seq, event_name, trace_id, occurred_at, payload
		FROM run_events
		WHERE run_id = $1
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
		WHERE run_id = $1
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

// --------------------
// Internal helpers
// --------------------

func (s *Service) getRunByRequestKey(ctx context.Context, requestKey string) (Run, bool, *APIError) {
	var out Run
	var dbState string
	var rk *string

	tctx, cancel := withTimeout(ctx, 5*time.Second)
	defer cancel()

	err := s.db.QueryRow(tctx, `
		SELECT
			run_id, trace_id, project_id, policy_version, pipeline_version,
			state, created_at, updated_at, request_key
		FROM runs
		WHERE request_key = $1
	`, requestKey).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID, &out.PolicyVersion, &out.PipelineVersion,
		&dbState, &out.CreatedAt, &out.UpdatedAt, &rk,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "idempotency lookup failed"}
	}

	out.RunID = normalizeRunID(out.RunID)
	out.TraceID = normalizeTraceID(out.TraceID)
	out.RequestKey = rk

	out.State = normalizeRunState(dbState)
	out.Status, out.Result = derivePublicStatusAndResult(out.State)

	return out, true, nil
}

func isPgUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "23505"
	}
	return false
}

// isPgUniqueViolationOnRequestKey returns true only when the unique violation is likely caused by request_key uniqueness.
// This avoids mis-classifying run_id PK collision (extremely rare) as an idempotency hit.
func isPgUniqueViolationOnRequestKey(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code != "23505" {
			return false
		}
		cn := strings.ToLower(strings.TrimSpace(pe.ConstraintName))
		// Heuristic: constraint/index name contains request_key
		if strings.Contains(cn, "request_key") {
			return true
		}
		// If constraint name isn't available, fall back to generic unique violation
		// ONLY when request_key is non-empty at the call site.
		if cn == "" {
			return true
		}
	}
	return false
}

func newTraceID() string {
	// 16 bytes => 32 hex chars
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "trace-" + ulid.Make().String()
	}
	return hex.EncodeToString(b)
}