package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// ------------------------------------------------------------
// POST /v1/runs   (official)
// POST /v1/analyze (legacy alias)
// Both call handleCreateRun with different source.
// ------------------------------------------------------------

func handleCreateRun(w http.ResponseWriter, req *http.Request, db *pgxpool.Pool, source string) {
	tid := TraceIDFromContext(req.Context())

	// defaults from env (contract)
	defaultPolicy := getenvDefault("AK_POLICY_VERSION", "v3")
	defaultPipeline := getenvDefault("AK_PIPELINE_VERSION", "v3")

	// Decode input: /v1/analyze and /v1/runs use same body shape
	var in CreateRunReq
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		WriteError(w, 400, "BadRequest", "invalid json", tid)
		return
	}

	// Normalize / defaults
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)

	if in.ProjectID == "" {
		in.ProjectID = "default"
	}
	if in.PolicyVersion == "" {
		in.PolicyVersion = defaultPolicy
	}
	if in.PipelineVersion == "" {
		in.PipelineVersion = defaultPipeline
	}

	mode := 0
	if in.Mode != nil {
		mode = *in.Mode
	}

	// Optional idempotency key
	requestKey := ReadIdempotencyKey(req)

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	// If request_key already exists -> idempotent reuse (world-standard)
	if requestKey != "" {
		var rid, rtid, st string
		err := db.QueryRow(ctx, `
			SELECT run_id, trace_id, status
			FROM runs
			WHERE request_key = $1
		`, requestKey).Scan(&rid, &rtid, &st)

		if err == nil {
			// Response trace header uses current request tid; body trace_id is run's authoritative trace_id.
			WriteJSON(w, 202, CreateRunResp{RunID: rid, TraceID: rtid, Status: st, Note: "idempotent_reuse"}, tid)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			logDBErr(tid, "idempotency lookup failed", err)
			WriteError(w, 500, "DbError", "idempotency lookup failed", tid)
			return
		}
	}

	// ---- Tx: runs + run_events(enqueued) MUST commit together ----
	// Use BeginTx for clearer semantics and future options.
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logDBErr(tid, "begin failed", err)
		WriteError(w, 500, "DbError", "begin failed", tid)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	runID := ulid.Make().String()

	// Insert runs
	_, err = tx.Exec(ctx, `
		INSERT INTO runs(run_id, trace_id, project_id, policy_version, pipeline_version, status, request_key)
		VALUES ($1,$2,$3,$4,$5,'queued',$6)
	`, runID, tid, in.ProjectID, in.PolicyVersion, in.PipelineVersion, NullableText(requestKey))
	if err != nil {
		// If request_key race collided, fetch and reuse
		if requestKey != "" {
			var rid, rtid, st string
			err2 := db.QueryRow(ctx, `
				SELECT run_id, trace_id, status
				FROM runs
				WHERE request_key = $1
			`, requestKey).Scan(&rid, &rtid, &st)

			if err2 == nil {
				WriteJSON(w, 202, CreateRunResp{RunID: rid, TraceID: rtid, Status: st, Note: "idempotent_race_reuse"}, tid)
				return
			}
			logDBErr(tid, "idempotency race reuse lookup failed", err2)
		}

		// Log richer PG error fields if possible
		logPGErr(tid, "insert runs failed", err)
		WriteError(w, 500, "DbError", "insert runs failed", tid)
		return
	}

	// event_seq must be serialized. For create: event_seq=1 always (enqueued).
	payload := map[string]any{
		"project_id":       in.ProjectID,
		"policy_version":   in.PolicyVersion,
		"pipeline_version": in.PipelineVersion,
		"mode":             mode,   // v3.2
		"source":           source, // observability
	}
	pb, _ := json.Marshal(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,1,'run.enqueued',$3::jsonb)
	`, runID, tid, string(pb))
	if err != nil {
		logPGErr(tid, "insert run_events failed", err)
		WriteError(w, 500, "DbError", "insert run_events failed", tid)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logDBErr(tid, "commit failed", err)
		WriteError(w, 500, "DbError", "commit failed", tid)
		return
	}

	// Response contract
	WriteJSON(w, 202, CreateRunResp{RunID: runID, TraceID: tid, Status: "queued"}, tid)
}

// ------------------------------------------------------------
// GET /v1/runs/{run_id}
// ------------------------------------------------------------

func handleGetRun(w http.ResponseWriter, req *http.Request, db *pgxpool.Pool) {
	tid := TraceIDFromContext(req.Context())
	runID := strings.TrimSpace(chi.URLParam(req, "run_id"))
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	var out RunRow
	err := db.QueryRow(ctx, `
		SELECT run_id, trace_id, project_id, policy_version, pipeline_version, status, created_at, updated_at
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID, &out.PolicyVersion, &out.PipelineVer,
		&out.Status, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		logDBErr(tid, "get run failed", err)
		// NotFound と DbError の区別を厳密にするなら pgx.ErrNoRows を見る
		if errors.Is(err, pgx.ErrNoRows) {
			WriteError(w, 404, "NotFound", "run not found", tid)
			return
		}
		WriteError(w, 500, "DbError", "query failed", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}

// ------------------------------------------------------------
// GET /v1/runs/{run_id}/events
// ------------------------------------------------------------

func handleGetRunEvents(w http.ResponseWriter, req *http.Request, db *pgxpool.Pool) {
	tid := TraceIDFromContext(req.Context())
	runID := strings.TrimSpace(chi.URLParam(req, "run_id"))
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT event_seq, event_name, trace_id, occurred_at, payload
		FROM run_events
		WHERE run_id = $1
		ORDER BY event_seq ASC
	`, runID)
	if err != nil {
		logDBErr(tid, "query run_events failed", err)
		WriteError(w, 500, "DbError", "query failed", tid)
		return
	}
	defer rows.Close()

	type ev struct {
		EventSeq   int64           `json:"event_seq"`
		EventName  string          `json:"event_name"`
		TraceID    string          `json:"trace_id"`
		OccurredAt time.Time       `json:"occurred_at"`
		Payload    json.RawMessage `json:"payload"`
	}

	out := struct {
		RunID  string `json:"run_id"`
		Events []ev   `json:"events"`
	}{
		RunID:  runID,
		Events: []ev{},
	}

	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.EventSeq, &e.EventName, &e.TraceID, &e.OccurredAt, &e.Payload); err != nil {
			logDBErr(tid, "scan run_events failed", err)
			WriteError(w, 500, "DbError", "scan failed", tid)
			return
		}
		out.Events = append(out.Events, e)
	}
	if err := rows.Err(); err != nil {
		logDBErr(tid, "rows run_events failed", err)
		WriteError(w, 500, "DbError", "rows error", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}

// ------------------------------------------------------------
// GET /v1/runs/{run_id}/artifacts
// ------------------------------------------------------------

func handleGetRunArtifacts(w http.ResponseWriter, req *http.Request, db *pgxpool.Pool) {
	tid := TraceIDFromContext(req.Context())
	runID := strings.TrimSpace(chi.URLParam(req, "run_id"))
	if runID == "" {
		WriteError(w, 400, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT artifact_kind, content_json, created_at, updated_at
		FROM run_artifacts
		WHERE run_id = $1
		ORDER BY artifact_kind ASC
	`, runID)
	if err != nil {
		logDBErr(tid, "query run_artifacts failed", err)
		WriteError(w, 500, "DbError", "query failed", tid)
		return
	}
	defer rows.Close()

	type a struct {
		Kind      string          `json:"artifact_kind"`
		Content   json.RawMessage `json:"content_json"`
		CreatedAt time.Time       `json:"created_at"`
		UpdatedAt time.Time       `json:"updated_at"`
	}

	out := struct {
		RunID     string `json:"run_id"`
		Artifacts []a    `json:"artifacts"`
	}{RunID: runID, Artifacts: []a{}}

	for rows.Next() {
		var x a
		if err := rows.Scan(&x.Kind, &x.Content, &x.CreatedAt, &x.UpdatedAt); err != nil {
			logDBErr(tid, "scan run_artifacts failed", err)
			WriteError(w, 500, "DbError", "scan failed", tid)
			return
		}
		out.Artifacts = append(out.Artifacts, x)
	}
	if err := rows.Err(); err != nil {
		logDBErr(tid, "rows run_artifacts failed", err)
		WriteError(w, 500, "DbError", "rows error", tid)
		return
	}

	WriteJSON(w, 200, out, tid)
}

// ------------------------------------------------------------
// helpers (local)
// ------------------------------------------------------------

func getenvDefault(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

// Optional: you can later use this to parse mode from query etc.
func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

// logDBErr: keep this local if your router.go expects it.
// If you already have logDBErr in router.go, delete one side and keep only one.
func logDBErr(tid, label string, err error) {
	if err == nil {
		return
	}
	log.Printf("[DB][trace=%s] %s: %v", tid, label, err)
}

// logPGErr prints richer pgconn.PgError information when available.
// This is extremely useful for the kind of "begin failed / query failed" debugging you hit.
func logPGErr(tid, label string, err error) {
	if err == nil {
		return
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		log.Printf("[DB][trace=%s] %s: %s (code=%s) detail=%s where=%s constraint=%s schema=%s table=%s column=%s",
			tid, label, pe.Message, pe.Code, pe.Detail, pe.Where, pe.ConstraintName, pe.SchemaName, pe.TableName, pe.ColumnName)
		return
	}
	logDBErr(tid, label, err)
}