package adminapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"example.com/pisag_go/httpx"
	"example.com/pisag_go/postgres"
)

type Server struct {
	Ops *postgres.DiscoveryOpsRepo
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// simple X-Admin-Key gate (dev: empty => allow)
	if k := strings.TrimSpace(os.Getenv("AK_ADMIN_X_ADMIN_KEY")); k != "" {
		if strings.TrimSpace(r.Header.Get("X-Admin-Key")) != k {
			httpx.WriteError(w, 403, "forbidden", "invalid admin key", httpx.TraceIDFromContext(r.Context()))
			return
		}
	}

	base := "/api/admin/atlaskernel/discovery-ops"
	if !strings.HasPrefix(r.URL.Path, base) {
		httpx.WriteError(w, 404, "not_found", "not found", httpx.TraceIDFromContext(r.Context()))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, base)
	path = strings.Trim(path, "/")

	// /candidates ...
	segs := []string{}
	if path != "" {
		segs = strings.Split(path, "/")
	}

	traceID := httpx.TraceIDFromContext(r.Context())

	// GET /candidates
	if r.Method == http.MethodGet && len(segs) == 1 && segs[0] == "candidates" {
		projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
		mode := strings.TrimSpace(r.URL.Query().Get("mode"))
		typ := strings.TrimSpace(r.URL.Query().Get("type"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		onlyDue := strings.TrimSpace(r.URL.Query().Get("only_due")) == "1"
		limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))

		items, err := s.Ops.ListCandidates(r.Context(), postgres.ListParams{
			ProjectID: projectID,
			Mode:      mode,
			Type:      typ,
			Status:    status,
			Q:         q,
			OnlyDue:   onlyDue,
			Limit:     limit,
		})
		if err != nil {
			httpx.WriteError(w, 400, "bad_request", err.Error(), traceID)
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"items": items, "count": len(items)}, traceID)
		return
	}

	// /candidates/{id}
	if len(segs) >= 2 && segs[0] == "candidates" {
		id, err := strconv.ParseInt(segs[1], 10, 64)
		if err != nil || id <= 0 {
			httpx.WriteError(w, 400, "bad_request", "invalid id", traceID)
			return
		}

		// GET /candidates/{id}
		if r.Method == http.MethodGet && len(segs) == 2 {
			d, err := s.Ops.GetCandidate(r.Context(), id)
			if err != nil {
				httpx.WriteError(w, 404, "not_found", err.Error(), traceID)
				return
			}
			httpx.WriteJSON(w, 200, d, traceID)
			return
		}

		// GET /candidates/{id}/events
		if r.Method == http.MethodGet && len(segs) == 3 && segs[2] == "events" {
			limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
			ev, err := s.Ops.ListEvents(r.Context(), id, limit)
			if err != nil {
				httpx.WriteError(w, 500, "db_error", err.Error(), traceID)
				return
			}
			httpx.WriteJSON(w, 200, ev, traceID)
			return
		}

		// POST actions
		if r.Method == http.MethodPost && len(segs) == 3 {
			var body struct {
				ProjectID string `json:"project_id"`
				TraceID   string `json:"trace_id"`
				RunID     string `json:"run_id"`  // optional; if empty we fall back to candidate.run_id
				Reason    string `json:"reason"`  // optional
			}
			if err := httpx.DecodeJSON(r, &body); err != nil {
				httpx.WriteError(w, 400, "bad_request", err.Error(), traceID)
				return
			}
			if strings.TrimSpace(body.TraceID) == "" {
				body.TraceID = traceID
			}
			body.ProjectID = strings.TrimSpace(body.ProjectID)
			if body.ProjectID == "" {
				httpx.WriteError(w, 400, "bad_request", "project_id is required", traceID)
				return
			}

			// run_id: use payload or read from candidate
			runID := strings.TrimSpace(body.RunID)
			if runID == "" {
				d, err := s.Ops.GetCandidate(r.Context(), id)
				if err == nil {
					runID = d.RunID
				}
			}
			if runID == "" {
				httpx.WriteError(w, 400, "bad_request", "run_id is required", traceID)
				return
			}

			action := segs[2]
			var aerr error
			switch action {
			case "requeue-review":
				aerr = s.Ops.RequeueReview(r.Context(), body.ProjectID, id, body.TraceID, runID, body.Reason)
			case "retry":
				aerr = s.Ops.RetryNow(r.Context(), body.ProjectID, id, body.TraceID, runID)
			case "apply-retry":
				aerr = s.Ops.ApplyRetryNow(r.Context(), body.ProjectID, id, body.TraceID, runID)
			case "archive":
				reason := body.Reason
				if strings.TrimSpace(reason) == "" {
					reason = "manual"
				}
				aerr = s.Ops.Archive(r.Context(), body.ProjectID, id, body.TraceID, runID, reason)
			case "unarchive":
				reason := body.Reason
				if strings.TrimSpace(reason) == "" {
					reason = "manual"
				}
				aerr = s.Ops.Unarchive(r.Context(), body.ProjectID, id, body.TraceID, runID, reason)
			default:
				httpx.WriteError(w, 404, "not_found", "unknown action", traceID)
				return
			}

			if aerr != nil {
				httpx.WriteError(w, 400, "action_failed", aerr.Error(), traceID)
				return
			}
			httpx.WriteJSON(w, 200, map[string]any{"ok": true}, traceID)
			return
		}
	}

	httpx.WriteError(w, 404, "not_found", "not found", traceID)
}

func JSON(w http.ResponseWriter, status int, v any, traceID string) {
	w.Header().Set("Content-Type", "application/json")
	if traceID != "" {
		w.Header().Set(httpx.TraceHeader, traceID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}