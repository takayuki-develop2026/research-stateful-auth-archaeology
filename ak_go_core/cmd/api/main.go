package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

const traceHeader = "X-Trace-Id"

type ctxKey string

const traceCtxKey ctxKey = "trace_id"

type errorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

type analyzeReq struct {
	ProjectID       string `json:"project_id"`
	PolicyVersion   string `json:"policy_version"`
	PipelineVersion string `json:"pipeline_version"`
}

func newTraceID() string {
	// 128-bit random hex (32 chars). Simple and header-friendly.
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitizeTraceID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// allow [a-zA-Z0-9-_.:] up to 128 chars.
	if len(v) > 128 {
		return ""
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return v
}

func traceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceCtxKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, body any, traceID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if traceID != "" {
		w.Header().Set(traceHeader, traceID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, typ, msg, traceID string) {
	var env errorEnvelope
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	writeJSON(w, status, env, traceID)
}

func nullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func main() {
	// ----------------------------
	// DB init (v3 SoT)
	// ----------------------------
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" {
		log.Fatal("AK_DB_DSN is required")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	// Defaults from env (v3 contract)
	defaultPolicy := os.Getenv("AK_POLICY_VERSION")
	if defaultPolicy == "" {
		defaultPolicy = "v3"
	}
	defaultPipeline := os.Getenv("AK_PIPELINE_VERSION")
	if defaultPipeline == "" {
		defaultPipeline = "v3"
	}

	r := chi.NewRouter()

	// Trace middleware: always return X-Trace-Id (use incoming if valid; else generate)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			tid := sanitizeTraceID(req.Header.Get(traceHeader))
			if tid == "" {
				tid = newTraceID()
			}
			ctx := context.WithValue(req.Context(), traceCtxKey, tid)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	// Health
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		tid := traceIDFromContext(req.Context())
		writeJSON(w, 200, map[string]any{
			"ok":   true,
			"ts":   time.Now().UTC().Format(time.RFC3339Nano),
			"node": "ak-go-core",
		}, tid)
	})

	// v3 entry: create (or reuse) run + append run.enqueued event, return 202
r.Post("/v1/analyze", func(w http.ResponseWriter, req *http.Request) {
	tid := traceIDFromContext(req.Context())

	var in analyzeReq
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeError(w, 400, "BadRequest", "invalid json", tid)
		return
	}
	if in.ProjectID == "" {
		in.ProjectID = "default"
	}
	if in.PolicyVersion == "" {
		in.PolicyVersion = defaultPolicy
	}
	if in.PipelineVersion == "" {
		in.PipelineVersion = defaultPipeline
	}

	// Optional idempotency key
	requestKey := strings.TrimSpace(req.Header.Get("Idempotency-Key"))
	if requestKey == "" {
		requestKey = strings.TrimSpace(req.Header.Get("X-Idempotency-Key"))
	}
	if len(requestKey) > 256 {
		// keep conservative
		requestKey = ""
	}

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	// If request_key is provided and already exists, return the existing run (idempotent)
	if requestKey != "" {
		var existingRunID, existingTraceID, existingStatus string
		err := db.QueryRow(ctx, `
			SELECT run_id, trace_id, status
			FROM runs
			WHERE request_key = $1
		`, requestKey).Scan(&existingRunID, &existingTraceID, &existingStatus)

		if err == nil {
			// Idempotent response: return existing run_id (202 is fine)
			writeJSON(w, 202, map[string]any{
				"run_id":   existingRunID,
				"trace_id": existingTraceID, // original trace_id (authoritative for that run)
				"status":   existingStatus,
				"note":     "idempotent_reuse",
			}, tid) // response trace header still uses current tid
			return
		}
		// if not found, proceed to create
	}

	runID := ulid.Make().String()

	tx, err := db.Begin(ctx)
	if err != nil {
		writeError(w, 500, "DbError", "begin failed", tid)
		return
	}
	defer tx.Rollback(ctx)

	// Create run
	_, err = tx.Exec(ctx, `
		INSERT INTO runs(run_id, trace_id, project_id, policy_version, pipeline_version, status, request_key)
		VALUES ($1,$2,$3,$4,$5,'queued',$6)
	`, runID, tid, in.ProjectID, in.PolicyVersion, in.PipelineVersion, nullableText(requestKey))
	if err != nil {
		// If request_key collided (race), fetch and return existing
		if requestKey != "" {
			var existingRunID, existingTraceID, existingStatus string
			if err2 := db.QueryRow(ctx, `
				SELECT run_id, trace_id, status
				FROM runs
				WHERE request_key = $1
			`, requestKey).Scan(&existingRunID, &existingTraceID, &existingStatus); err2 == nil {
				writeJSON(w, 202, map[string]any{
					"run_id":   existingRunID,
					"trace_id": existingTraceID,
					"status":   existingStatus,
					"note":     "idempotent_race_reuse",
				}, tid)
				return
			}
		}
		writeError(w, 500, "DbError", "insert runs failed", tid)
		return
	}

	// Append event seq=1: run.enqueued
	payload := map[string]any{
		"project_id":       in.ProjectID,
		"policy_version":   in.PolicyVersion,
		"pipeline_version": in.PipelineVersion,
	}
	pb, _ := json.Marshal(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,1,'run.enqueued',$3::jsonb)
	`, runID, tid, string(pb))
	if err != nil {
		writeError(w, 500, "DbError", "insert run_events failed", tid)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, 500, "DbError", "commit failed", tid)
		return
	}

	// return 202 Accepted
	writeJSON(w, 202, map[string]any{
		"run_id":   runID,
		"trace_id": tid,
		"status":   "queued",
	}, tid)
	})

	r.Get("/v1/runs/{run_id}", func(w http.ResponseWriter, req *http.Request) {
	tid := traceIDFromContext(req.Context())
	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		writeError(w, 400, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	var out struct {
		RunID         string    `json:"run_id"`
		TraceID       string    `json:"trace_id"`
		ProjectID     string    `json:"project_id"`
		PolicyVersion string    `json:"policy_version"`
		PipelineVer   string    `json:"pipeline_version"`
		Status        string    `json:"status"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}

	err := db.QueryRow(ctx, `
		SELECT run_id, trace_id, project_id, policy_version, pipeline_version, status, created_at, updated_at
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(
		&out.RunID, &out.TraceID, &out.ProjectID, &out.PolicyVersion, &out.PipelineVer,
		&out.Status, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		writeError(w, 404, "NotFound", "run not found", tid)
		return
	}

	writeJSON(w, 200, out, tid)
	})

	r.Get("/v1/runs/{run_id}/events", func(w http.ResponseWriter, req *http.Request) {
	tid := traceIDFromContext(req.Context())
	runID := chi.URLParam(req, "run_id")
	if runID == "" {
		writeError(w, 400, "BadRequest", "run_id is required", tid)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT event_seq, event_name, trace_id, occurred_at, payload
		FROM run_events
		WHERE run_id = $1
		ORDER BY event_seq ASC
	`, runID)
	if err != nil {
		writeError(w, 500, "DbError", "query failed", tid)
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
			writeError(w, 500, "DbError", "scan failed", tid)
			return
		}
		out.Events = append(out.Events, e)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "DbError", "rows error", tid)
		return
	}

	writeJSON(w, 200, out, tid)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	addr := ":" + port

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("AK Go Core listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-done
	log.Printf("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Printf("bye")
}
