package httpx

import (
	"example.com/ak_go_core/internal/app/runs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

func NewRouter(db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Trace middleware: always return X-Trace-Id
	r.Use(TraceMiddleware)

	// ✅ ここを追加：panic でプロセスが死なないようにする
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			tid := TraceIDFromContext(req.Context())
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[PANIC][trace=%s] %v\n%s", tid, rec, debug.Stack())
					// ここで JSON 500 を返して “落ちない”
					WriteError(w, 500, "Panic", "internal server error", tid)
				}
			}()
			next.ServeHTTP(w, req)
		})
	})

	// wiring
	// NOTE: runs.ServiceConfig が無いなら、ここは一旦コンパイル通る形に合わせてください。
	// すでに build/test が通っているとのことなので、あなたの runs.NewService のシグネチャに合わせています。
	runsSvc := runs.NewService(db, runs.ServiceConfig{
		DefaultPolicyVersion:   getenvDefault("AK_POLICY_VERSION", "v3"),
		DefaultPipelineVersion: getenvDefault("AK_PIPELINE_VERSION", "v3"),
	})
	h := NewHandlers(runsSvc)

	// Health
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		tid := TraceIDFromContext(req.Context())
		WriteJSON(w, 200, map[string]any{
			"ok":   true,
			"ts":   time.Now().UTC().Format(time.RFC3339Nano),
			"node": "ak-go-core",
		}, tid)
	})

	// v1: create run
	r.Post("/v1/runs", h.PostRuns)

	// legacy: analyze => same logic
	r.Post("/v1/analyze", h.PostAnalyze)

	// reads
	r.Get("/v1/runs/{run_id}", h.GetRun)
	r.Get("/v1/runs/{run_id}/events", h.GetRunEvents)
	r.Get("/v1/runs/{run_id}/artifacts", h.GetRunArtifacts)

	// --- P0: always JSON + X-Trace-Id for routing errors ---
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		tid := TraceIDFromContext(req.Context())
		WriteError(w, 404, "NotFound", "route not found", tid)
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		tid := TraceIDFromContext(req.Context())
		WriteError(w, 405, "MethodNotAllowed", "method not allowed", tid)
	})

	return r
}

func getenvDefault(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
