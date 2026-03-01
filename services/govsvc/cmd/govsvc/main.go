package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultPort = "9012"
	traceHeader = "X-Trace-Id"
	serviceName = "govsvc"

	// dev storage (bind-mounted)
	defaultEvidenceDir = "/var/govsvc/evidence"
)

type CreatePolicySetRequest struct {
	ProjectID   string `json:"project_id"` // text like akproj_...
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PublishPolicyRequest struct {
	ProjectID      string `json:"project_id"`
	PublishedBy    string `json:"published_by"`
	PublishReason  string `json:"publish_reason"`
	Confirm        bool   `json:"confirm"`
	IncidentID     string `json:"incident_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RollbackPolicyRequest struct {
	ProjectID      string `json:"project_id"`
	ToVersionID    string `json:"to_version_id"` // must be published
	Reason         string `json:"reason"`
	Confirm        bool   `json:"confirm"`
	IncidentID     string `json:"incident_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RetirePolicyRequest struct {
	ProjectID      string `json:"project_id"`
	VersionID      string `json:"version_id"` // must be published
	Reason         string `json:"reason"`
	Confirm        bool   `json:"confirm"`
	IdempotencyKey string `json:"idempotency_key"`
}

func main() {
	port := os.Getenv("GOVSVC_PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()

	// ------------------------------------------------------------
	// /health
	// ------------------------------------------------------------
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"service":  serviceName,
			"now":      time.Now().Format(time.RFC3339Nano),
			"trace_id": traceID,
		})
	})

	// ------------------------------------------------------------
	// /health/db
	// ------------------------------------------------------------
	mux.HandleFunc("/health/db", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		conn, err := openDB(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ok": false, "service": serviceName, "trace_id": traceID,
				"error": "connect_failed",
				"detail": err.Error(),
			})
			return
		}
		defer conn.Close(r.Context())

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var one int
		if err := conn.QueryRow(ctx, "select 1").Scan(&one); err != nil || one != 1 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ok": false, "service": serviceName, "trace_id": traceID,
				"error": "query_failed",
				"detail": errString(err),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"db": "ak_postgres", "ok": true, "select_1": one,
			"service": serviceName, "trace_id": traceID,
		})
	})

	// ------------------------------------------------------------
	// /v1/policies/sets  (GET list, POST create)
	// ------------------------------------------------------------
	mux.HandleFunc("/v1/policies/sets", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		if r.Method == http.MethodGet {
			projectID := r.URL.Query().Get("project_id")
			if projectID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "missing_project_id", "trace_id": traceID,
				})
				return
			}

			conn, err := openDB(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"error": "connect_failed", "trace_id": traceID,
					"detail": err.Error(),
				})
				return
			}
			defer conn.Close(r.Context())

			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			rows, err := conn.Query(ctx, `
				SELECT id::text, project_key, name, description,
				       COALESCE(active_published_version_id::text,''), status,
				       created_at, updated_at
				  FROM gov_policy.policy_sets_list_v12($1::text)
			`, projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"error": "db_call_failed", "trace_id": traceID,
					"detail": err.Error(),
				})
				return
			}
			defer rows.Close()

			type item struct {
				ID                     string    `json:"id"`
				ProjectID              string    `json:"project_id"`
				Name                   string    `json:"name"`
				Description            *string   `json:"description"`
				ActivePublishedVersion string    `json:"active_published_version_id"`
				Status                 string    `json:"status"`
				CreatedAt              time.Time `json:"created_at"`
				UpdatedAt              time.Time `json:"updated_at"`
			}

			var out []item
			for rows.Next() {
				var it item
				var desc *string
				var activeVer string
				if err := rows.Scan(&it.ID, &it.ProjectID, &it.Name, &desc, &activeVer, &it.Status, &it.CreatedAt, &it.UpdatedAt); err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error": "scan_failed", "trace_id": traceID,
						"detail": err.Error(),
					})
					return
				}
				it.Description = desc
				it.ActivePublishedVersion = activeVer
				out = append(out, it)
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"policy_sets": out,
				"trace_id":    traceID,
			})
			return
		}

		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
				"error": "method_not_allowed", "trace_id": traceID,
			})
			return
		}

		var req CreatePolicySetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_json", "trace_id": traceID,
			})
			return
		}
		if req.ProjectID == "" || req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "missing_fields",
				"detail": "project_id and name are required",
				"trace_id": traceID,
			})
			return
		}

		conn, err := openDB(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "connect_failed", "trace_id": traceID,
				"detail": err.Error(),
			})
			return
		}
		defer conn.Close(r.Context())

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var policySetID *string
		err = conn.QueryRow(ctx, `
			SELECT gov_policy.policy_set_create_v12b(
				$1::text,
				$2::text,
				$3::text,
				$4::text
			)::text
		`, req.ProjectID, req.Name, nullIfEmpty(req.Description), traceID).Scan(&policySetID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "db_call_failed", "trace_id": traceID,
				"detail": err.Error(),
			})
			return
		}
		if policySetID == nil || *policySetID == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "db_returned_null", "trace_id": traceID,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"policy_set_id": *policySetID,
			"trace_id":      traceID,
		})
	})

	// ------------------------------------------------------------
	// POST /v1/policies/sets/{policy_set_id}/publish
	// POST /v1/policies/sets/{policy_set_id}/rollback
	// POST /v1/policies/sets/{policy_set_id}/retire
	// GET  /v1/policies/sets/{policy_set_id}/active
	// ------------------------------------------------------------
	mux.HandleFunc("/v1/policies/sets/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		path := strings.TrimPrefix(r.URL.Path, "/v1/policies/sets/")
		parts := strings.Split(strings.Trim(path, "/"), "/")

		if len(parts) < 2 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
			return
		}

		policySetID := parts[0]
		action := parts[1]

		// --------------------------------------------------------
		// GET active
		// --------------------------------------------------------
		if action == "active" {
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}
			projectID := r.URL.Query().Get("project_id")
			if projectID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_project_id", "trace_id": traceID})
				return
			}

			conn, err := openDB(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}
			defer conn.Close(r.Context())

			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			var setID string
			var activeID *string
			err = conn.QueryRow(ctx, `
				SELECT policy_set_id::text, active_published_version_id::text
				  FROM gov_policy.policy_set_active_v12($1::text, $2::uuid)
			`, projectID, policySetID).Scan(&setID, &activeID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"policy_set_id":               setID,
				"active_published_version_id": activeID,
				"trace_id":                    traceID,
			})
			return
		}

		// --------------------------------------------------------
		// POST publish
		// --------------------------------------------------------
		if action == "publish" {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}

			var req PublishPolicyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
				return
			}

			if !req.Confirm {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
				return
			}
			if req.ProjectID == "" || req.PublishedBy == "" || req.PublishReason == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "missing_fields",
					"detail": "project_id, published_by, publish_reason are required",
					"trace_id": traceID,
				})
				return
			}
			if req.IdempotencyKey == "" {
				req.IdempotencyKey = "pub-" + newTraceID()
			}

			conn, err := openDB(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}
			defer conn.Close(r.Context())

			ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
			defer cancel()

			compiled := map[string]any{
				"policy_set_id": policySetID,
				"generated_at":  time.Now().Format(time.RFC3339Nano),
				"rules":         []any{},
			}
			compiledBytes, _ := json.Marshal(compiled)

			evidenceDir := os.Getenv("GOVSVC_EVIDENCE_DIR")
			if evidenceDir == "" {
				evidenceDir = defaultEvidenceDir
			}
			if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_dir_unavailable", "detail": err.Error(), "trace_id": traceID})
				return
			}

			contentSha := sha256Hex(compiledBytes)
			fileName := "compiled_policy_" + contentSha + ".json"
			filePath := filepath.Join(evidenceDir, fileName)
			if err := writeFileIfNotExists(filePath, compiledBytes); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_write_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}

			var evidenceRef string
			var foundExisting bool
			err = conn.QueryRow(ctx, `
				SELECT evidence_ref::text, found_existing
				  FROM public.evidence_register_v18(
				    $1::varchar, $2::varchar,
				    $3::varchar, $4::varchar,
				    $5::varchar, $6::varchar,
				    $7::varchar, $8::text,
				    $9::text, $10::bigint,
				    $11::varchar, $12::varchar,
				    $13::timestamptz,
				    $14::text
				  )
			`,
				req.ProjectID, traceID,
				"system", "govsvc",
				"text", "application/json",
				"generated", filePath,
				contentSha, int64(len(compiledBytes)),
				"ja", "standard",
				nil,
				"evi-"+req.IdempotencyKey,
			).Scan(&evidenceRef, &foundExisting)
			if err != nil || evidenceRef == "" {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "evidence_register_failed", "detail": errString(err), "trace_id": traceID})
				return
			}

			var policyVersionID *string
			err = conn.QueryRow(ctx, `
				SELECT gov_policy.policy_version_publish_v12(
					$1::uuid,
					$2::uuid,
					$3::char(64),
					$4::text,
					$5::text,
					$6::text
				)::text
			`, policySetID, evidenceRef, contentSha, req.PublishedBy, req.PublishReason, traceID).Scan(&policyVersionID)

			if err != nil || policyVersionID == nil || *policyVersionID == "" {
				pubID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "publish", "", "", req.PublishedBy, req.PublishReason, req.IncidentID, "failed_recorded", traceID, req.IdempotencyKey, "")
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "failed_recorded",
					"publication_id": pubID,
					"trace_id": traceID,
					"detail": errString(err),
				})
				return
			}

			publicationID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "publish", "", *policyVersionID, req.PublishedBy, req.PublishReason, req.IncidentID, "succeeded", traceID, req.IdempotencyKey, "")
			writeJSON(w, http.StatusOK, map[string]any{
				"policy_version_id":            *policyVersionID,
				"publication_id":               publicationID,
				"compiled_policy_evidence_ref": evidenceRef,
				"compiled_policy_checksum":     contentSha,
				"found_existing_evidence":      foundExisting,
				"trace_id": traceID,
			})
			return
		}

		// --------------------------------------------------------
		// POST rollback
		// --------------------------------------------------------
		if action == "rollback" {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}

			var req RollbackPolicyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
				return
			}
			if !req.Confirm {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
				return
			}
			if req.ProjectID == "" || req.ToVersionID == "" || req.Reason == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "missing_fields",
					"detail": "project_id, to_version_id, reason are required",
					"trace_id": traceID,
				})
				return
			}
			if req.IdempotencyKey == "" {
				req.IdempotencyKey = "rb-" + newTraceID()
			}

			conn, err := openDB(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}
			defer conn.Close(r.Context())

			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			var ok bool
			err = conn.QueryRow(ctx, `
				SELECT gov_policy.policy_set_rollback_v12(
					$1::uuid,
					$2::uuid,
					$3::text
				)
			`, policySetID, req.ToVersionID, traceID).Scan(&ok)
			if err != nil || !ok {
				pubID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "rollback", "", req.ToVersionID, "govsvc", req.Reason, req.IncidentID, "failed_recorded", traceID, req.IdempotencyKey, "")
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "failed_recorded",
					"publication_id": pubID,
					"trace_id": traceID,
					"detail": errString(err),
				})
				return
			}

			pubID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "rollback", "", req.ToVersionID, "govsvc", req.Reason, req.IncidentID, "succeeded", traceID, req.IdempotencyKey, "")
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "succeeded",
				"publication_id": pubID,
				"trace_id": traceID,
			})
			return
		}

		// --------------------------------------------------------
		// POST retire
		// --------------------------------------------------------
		if action == "retire" {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
				return
			}

			var req RetirePolicyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "trace_id": traceID})
				return
			}
			if !req.Confirm {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm_required", "trace_id": traceID})
				return
			}
			if req.ProjectID == "" || req.VersionID == "" || req.Reason == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "missing_fields",
					"detail": "project_id, version_id, reason are required",
					"trace_id": traceID,
				})
				return
			}
			if req.IdempotencyKey == "" {
				req.IdempotencyKey = "rt-" + newTraceID()
			}

			conn, err := openDB(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
				return
			}
			defer conn.Close(r.Context())

			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			var ok bool
			err = conn.QueryRow(ctx, `
				SELECT gov_policy.policy_version_retire_v12(
					$1::uuid,
					$2::uuid,
					$3::text
				)
			`, policySetID, req.VersionID, traceID).Scan(&ok)
			if err != nil || !ok {
				pubID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "retire", "", req.VersionID, "govsvc", req.Reason, "", "failed_recorded", traceID, req.IdempotencyKey, "")
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "failed_recorded",
					"publication_id": pubID,
					"trace_id": traceID,
					"detail": errString(err),
				})
				return
			}

			pubID := recordPublicationBestEffort(ctx, conn, req.ProjectID, policySetID, "retire", "", req.VersionID, "govsvc", req.Reason, "", "succeeded", traceID, req.IdempotencyKey, "")
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "succeeded",
				"publication_id": pubID,
				"trace_id": traceID,
			})
			return
		}

		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "trace_id": traceID})
	})

	// ------------------------------------------------------------
	// GET /v1/policies/versions/{id}
	// ------------------------------------------------------------
	mux.HandleFunc("/v1/policies/versions/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)

		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed", "trace_id": traceID})
			return
		}

		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_project_id", "trace_id": traceID})
			return
		}

		versionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/policies/versions/"), "/")
		if versionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_version_id", "trace_id": traceID})
			return
		}

		conn, err := openDB(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "connect_failed", "detail": err.Error(), "trace_id": traceID})
			return
		}
		defer conn.Close(r.Context())

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var (
			id            string
			policySetId   string
			versionNumber int
			status        string
			evidenceRef   string
			checksum      string
			publishedBy   string
			publishedAt   time.Time
			publishReason string
			prev          *string
		)

		err = conn.QueryRow(ctx, `
			SELECT id::text, policy_set_id::text, version_number, status,
			       compiled_policy_evidence_asset_id::text, compiled_policy_checksum::text,
			       published_by, published_at, publish_reason,
			       previous_version_id::text
			  FROM gov_policy.policy_version_get_v12($1::text, $2::uuid)
		`, projectID, versionID).Scan(
			&id, &policySetId, &versionNumber, &status,
			&evidenceRef, &checksum,
			&publishedBy, &publishedAt, &publishReason,
			&prev,
		)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db_call_failed", "detail": err.Error(), "trace_id": traceID})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":                           id,
			"policy_set_id":                policySetId,
			"version_number":               versionNumber,
			"status":                       status,
			"compiled_policy_evidence_ref": evidenceRef,
			"compiled_policy_checksum":     checksum,
			"published_by":                 publishedBy,
			"published_at":                 publishedAt.Format(time.RFC3339Nano),
			"publish_reason":               publishReason,
			"previous_version_id":          prev,
			"trace_id":                     traceID,
		})
	})

	// root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		traceID := ensureTraceID(w, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"service":  serviceName,
			"message":  "govsvc up",
			"trace_id": traceID,
		})
	})

	addr := "0.0.0.0:" + port
	log.Printf("[%s] listening on %s", serviceName, addr)
	if err := http.ListenAndServe(addr, withLogging(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func openDB(parent context.Context) (*pgx.Conn, error) {
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" {
		return nil, errors.New("AK_DB_DSN is empty")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	return pgx.Connect(ctx, dsn)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func ensureTraceID(w http.ResponseWriter, r *http.Request) string {
	v := r.Header.Get(traceHeader)
	if v == "" {
		v = newTraceID()
	}
	w.Header().Set(traceHeader, v)
	return v
}

func newTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writeFileIfNotExists(path string, b []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func recordPublicationBestEffort(
	ctx context.Context,
	conn *pgx.Conn,
	projectID string,
	policySetID string,
	action string, // publish|rollback|retire
	fromVersionID string,
	toVersionID string,
	triggeredBy string,
	reason string,
	incidentID string,
	status string, // succeeded|failed_recorded
	traceID string,
	idempotencyKey string,
	resultEvidenceRef string,
) string {
	var pubID *string
	_ = conn.QueryRow(ctx, `
		SELECT gov_policy.policy_publication_record_v12b(
			$1::text,
			$2::uuid,
			$3::text,
			$4::uuid,
			$5::uuid,
			$6::text,
			$7::text,
			$8::text,
			$9::text,
			$10::uuid,
			$11::text,
			$12::text
		)::text
	`,
		projectID,
		policySetID,
		action,
		nullUUID(fromVersionID),
		nullUUID(toVersionID),
		triggeredBy,
		reason,
		nullIfEmptyString(incidentID),
		status,
		nullUUID(resultEvidenceRef),
		traceID,
		idempotencyKey,
	).Scan(&pubID)

	if pubID == nil {
		return ""
	}
	return *pubID
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmptyString(s string) string {
	return s
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}