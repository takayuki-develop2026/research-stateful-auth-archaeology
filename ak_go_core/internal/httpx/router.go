package httpx

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"example.com/ak_go_core/internal/app/runs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
P0:
- Never crash the process on panic.
- Always return JSON error with X-Trace-Id (TraceMiddleware).
- If response already started, cannot overwrite -> log only.
*/

type responseRecorder struct {
	http.ResponseWriter
	wroteHeader bool
	statusCode  int
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.wroteHeader = true
		r.statusCode = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		// default status
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func NewRouter(db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Trace middleware: always return X-Trace-Id
	r.Use(TraceMiddleware)

	// P0: panic guard (do not let the process die)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			tid := TraceIDFromContext(req.Context())

			rr := &responseRecorder{ResponseWriter: w}

			defer func() {
				if rec := recover(); rec != nil {
					// P0: minimal fatal proof
					log.Printf("[RUN_FATAL][trace=%s] %v\n%s", tid, rec, debug.Stack())

					// If response already started, we can't change it safely
					if rr.wroteHeader {
						return
					}

					// Return JSON 500
					WriteError(rr, 500, "Panic", "internal server error", tid)
				}
			}()

			next.ServeHTTP(rr, req)
		})
	})

	// wiring
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
