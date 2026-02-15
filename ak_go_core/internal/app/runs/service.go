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

type CreateRunOutput struct {
	RunID   string `json:"run_id"`
	TraceID string `json:"trace_id"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}

type Run struct {
	RunID           string    `json:"run_id"`
	TraceID         string    `json:"trace_id"`
	ProjectID       string    `json:"project_id"`
	PolicyVersion   string    `json:"policy_version"`
	PipelineVersion string    `json:"pipeline_version"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	RequestKey      *string   `json:"request_key,omitempty"`
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

	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		traceID = newTraceID() // ✅ 必ず生成
	}

	// idempotency reuse (request_key)
	if requestKey != "" {
		var rid, rtid, st string
		err := s.db.QueryRow(ctx, `
			SELECT run_id, trace_id, status
			FROM runs
			WHERE request_key = $1
		`, requestKey).Scan(&rid, &rtid, &st)

		if err == nil {
			return CreateRunOutput{RunID: rid, TraceID: rtid, Status: st, Note: "idempotent_reuse"}, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "idempotency lookup failed"}
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "begin failed"}
	}
	defer tx.Rollback(ctx)

	runID := ulid.Make().String() // ✅ 26 chars

	var rk any = nil
	if requestKey != "" {
		rk = requestKey
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO runs(run_id, trace_id, project_id, policy_version, pipeline_version, status, request_key)
		VALUES ($1,$2,$3,$4,$5,'queued',$6)
	`, runID, traceID, projectID, policy, pipeline, rk)
	if err != nil {
		if requestKey != "" && isPgUniqueViolation(err) {
			var rid, rtid, st string
			err2 := s.db.QueryRow(ctx, `
				SELECT run_id, trace_id, status
				FROM runs
				WHERE request_key = $1
			`, requestKey).Scan(&rid, &rtid, &st)
			if err2 == nil {
				return CreateRunOutput{RunID: rid, TraceID: rtid, Status: st, Note: "idempotent_reuse"}, nil
			}
		}
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "insert runs failed"}
	}

	payload := map[string]any{
		"project_id":       projectID,
		"policy_version":   policy,
		"pipeline_version": pipeline,
		"mode":             mode,
		"source":           strings.TrimSpace(in.Source),
	}
	pb, _ := json.Marshal(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,1,'run.enqueued',$3::jsonb)
	`, runID, traceID, string(pb))
	if err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "insert run_events failed"}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateRunOutput{}, &APIError{HTTPStatus: 500, Type: "DbError", Message: "commit failed"}
	}

	return CreateRunOutput{RunID: runID, TraceID: traceID, Status: "queued"}, nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (Run, bool, *APIError) {
	var out Run
	var requestKey *string

	err := s.db.QueryRow(ctx, `
		SELECT run_id, trace_id, project_id, policy_version, pipeline_version, status, created_at, updated_at, request_key
		FROM runs
		WHERE run_id = $1
	`, strings.TrimSpace(runID)).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID, &out.PolicyVersion, &out.PipelineVersion,
		&out.Status, &out.CreatedAt, &out.UpdatedAt, &requestKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}
	out.RequestKey = requestKey
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

	rows, err := s.db.Query(ctx, `
		SELECT event_seq, event_name, trace_id, occurred_at, payload
		FROM run_events
		WHERE run_id = $1
		ORDER BY event_seq ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return RunEventsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}
	defer rows.Close()

	out := RunEventsOut{RunID: strings.TrimSpace(runID), Events: []RunEvent{}}
	for rows.Next() {
		var e RunEvent
		if err := rows.Scan(&e.EventSeq, &e.EventName, &e.TraceID, &e.OccurredAt, &e.Payload); err != nil {
			return RunEventsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "scan failed"}
		}
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

	rows, err := s.db.Query(ctx, `
		SELECT artifact_kind, content_json, created_at, updated_at
		FROM run_artifacts
		WHERE run_id = $1
		ORDER BY artifact_kind ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return RunArtifactsOut{}, false, &APIError{HTTPStatus: 500, Type: "DbError", Message: "query failed"}
	}
	defer rows.Close()

	out := RunArtifactsOut{RunID: strings.TrimSpace(runID), Artifacts: []RunArtifact{}}
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

func isPgUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "23505"
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