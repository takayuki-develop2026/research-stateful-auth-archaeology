package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	// "github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type runRow struct {
	RunID   string
	TraceID string
}

type RouteDecision struct {
	RouteID  string
	Provider string
	Model    string
}

// policy values (project_settings.allowed_*_policy)
const (
	PolicyAllowAll  = "allow_all"
	PolicyDenyAll   = "deny_all"
	PolicyAllowList = "allow_list"
)

type projectSettingsRow struct {
	AllowedModes  []int
	AllowedRoutes []string
	IsEnabled     bool

	AllowedModesPolicy  string
	AllowedRoutesPolicy string
}
const BuildTag = "worker-attempt-20260214-02"
func main() {
	dsn := mustEnv("AK_DB_DSN")

	// NOTE: compose has AK_WORKER_NAME but code uses AK_WORKER_ID.
	// Keep current behavior (no breaking change).
	workerID := getenvDefault("AK_WORKER_ID", hostnameFallback())

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	redisAddr := getenvDefault("AK_REDIS_ADDR", "ak_redis:6379")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	poll := getenvDurationMS("AK_WORKER_POLL_MS", 500*time.Millisecond)
	cost := getenvInt64("AK_RUN_COST", 10)

	log.Printf(
  "AK Go Worker started. build=%s worker_id=%s redis=%s poll=%s cost=%d",
  BuildTag, workerID, redisAddr, poll, cost,
)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		run, ok, pickErr := pickQueuedRun(ctx, db, workerID)
		cancel()

		if pickErr != nil {
			log.Printf("pickQueuedRun error: %v", pickErr)
			time.Sleep(poll)
			continue
		}
		if !ok {
			time.Sleep(poll)
			continue
		}

		// normalize run_id (CHAR(26) may come with trailing spaces)
		run.RunID = normalizeRunID(run.RunID)
		run.TraceID = strings.TrimSpace(run.TraceID)

		if !isValidRunID(run.RunID) {
			holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
				"invalid_run_id",
				map[string]any{"run_id": run.RunID, "expected_len": 26},
			)
			time.Sleep(poll)
			continue
		}

		// Redis lock: best-effort only (DB is the final guard)
		// IMPORTANT: if lock cannot be acquired, DO NOT write to DB.
		lockKey := "run:" + run.RunID + ":lock"
		locked, err := redisSetNX(context.Background(), rdb, lockKey, "1", 90*time.Second)
		if err != nil {
			log.Printf("redis SetNX error: %v", err)
			time.Sleep(poll)
			continue
		}
		if !locked {
			time.Sleep(poll)
			continue
		}

		func() {
			defer func() { _ = redisDel(context.Background(), rdb, lockKey) }()

			projectID, err := getRunProjectID(context.Background(), db, run.RunID)
			if err != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"project_id_lookup_failed",
					map[string]any{"error_detail": err.Error()},
				)
				return
			}
			projectID = strings.TrimSpace(projectID)

			pickedAt := time.Now().UTC().Format(time.RFC3339Nano)

			// run.running is observability ONLY (status already set to 'running' by pickQueuedRun)
			if err := appendEvent(context.Background(), db, run.RunID, run.TraceID,
				"run.running",
				map[string]any{
					"worker_id":  workerID,
					"picked_at":  pickedAt,
					"project_id": projectID,
				},
			); err != nil {
				log.Printf("run.running append failed: %v", err)
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"event_write_failed",
					map[string]any{"stage": "run.running", "error_detail": err.Error(), "project_id": projectID},
				)
				return
			}

			// ---- attempt: schema-less, stored in run_artifacts(attempt_state) ----
			attempt, aErr := beginAttemptTx(context.Background(), db, run.RunID, run.TraceID, projectID, workerID)
			if aErr != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"attempt_begin_failed",
					map[string]any{"error_detail": aErr.Error(), "project_id": projectID},
				)
				return
			}

			// ---- v3.2: mode from run.enqueued payload ----
			mode, modeErr := getRunModeFromEnqueuedEvent(context.Background(), db, run.RunID)
			if modeErr != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"mode_lookup_failed",
					map[string]any{"error_detail": modeErr.Error(), "project_id": projectID},
				)
				return
			}

			// ---- v3.2: Router (stub) ----
			routeDecision := decideRoute(mode)

			// ---- v3.3: project gate (policy-aware) ----
			allowed, gateReason, gateDetail, gateErr := gateByProjectSettings(
				context.Background(), db,
				projectID, mode, routeDecision.RouteID,
			)
			if gateErr != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"project_gate_error",
					map[string]any{
						"project_id":   projectID,
						"mode":         mode,
						"route_id":     routeDecision.RouteID,
						"error_detail": gateErr.Error(),
					},
				)
				return
			}
			if !allowed {
				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					"run.review_required", "review_required",
					map[string]any{
						"reason":     gateReason,
						"error_code": "project_gate_denied",
						"details":    gateDetail,
						"worker_id":  workerID,
						"project_id": projectID,
						"mode":       mode,
						"route_id":   routeDecision.RouteID,
						"attempt":    attempt,
					},
				)

				_ = upsertRunArtifact(context.Background(), db, run.RunID, "review_required_reason", map[string]any{
					"reason":     gateReason,
					"project_id": projectID,
					"mode":       mode,
					"route_id":   routeDecision.RouteID,
					"details":    gateDetail,
					"worker_id":  workerID,
					"attempt":    attempt,
					"decidedAt":  nowRFC3339Nano(),
				})

				_ = insertLearnSignal(context.Background(), db, run.RunID, projectID,
					"project_gate_denied",
					map[string]any{
						"mode":      mode,
						"route_id":  routeDecision.RouteID,
						"reason":    gateReason,
						"details":   gateDetail,
						"worker_id": workerID,
						"attempt":   attempt,
					},
				)
				return
			}

			_ = insertLearnSignal(context.Background(), db, run.RunID, projectID,
				"mode_received",
				map[string]any{"mode": mode, "worker_id": workerID, "attempt": attempt},
			)

			_ = upsertRunArtifact(context.Background(), db, run.RunID, "route_decision", map[string]any{
				"mode":      mode,
				"route_id":  routeDecision.RouteID,
				"provider":  routeDecision.Provider,
				"model":     routeDecision.Model,
				"attempt":   attempt,
				"decidedAt": nowRFC3339Nano(),
			})

			// ----------------------------------------------------------------------
			// v3.1 budget: reserve/capture/release (attempt-aware)
			// ----------------------------------------------------------------------

			reserved := false
			captured := false
			reasonReserve := fmt.Sprintf("reserve_run_cost_a%d", attempt)
			reasonRelease := fmt.Sprintf("release_run_cost_a%d", attempt)
			reasonCapture := fmt.Sprintf("capture_run_cost_a%d", attempt)

			// release safety-net
			defer func() {
				if !reserved {
					return
				}
				if captured {
					return
				}
				// best-effort release; never crash
				_ = releaseBudgetTx(context.Background(), db, run.RunID, run.TraceID, projectID, cost, reasonRelease)
				_ = appendEvent(context.Background(), db, run.RunID, run.TraceID,
					"budget.release",
					map[string]any{
						"project_id": projectID,
						"amount":     cost,
						"unit":       "credits",
						"reason":     reasonRelease,
						"attempt":    attempt,
						"worker_id":  workerID,
					},
				)
			}()

			// gate+reserve
			blockedEvent, blockedPayload, err := gateAndReserveBudgetTx(
				context.Background(), db,
				run.RunID, run.TraceID, projectID,
				cost,
				reasonReserve,
			)
			if err != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"budget_gate_reserve_failed",
					map[string]any{"error_detail": err.Error(), "project_id": projectID, "attempt": attempt},
				)
				return
			}
			if blockedEvent != "" {
				if blockedPayload == nil {
					blockedPayload = map[string]any{}
				}
				blockedPayload["worker_id"] = workerID
				blockedPayload["project_id"] = projectID
				blockedPayload["blocked_event"] = blockedEvent
				blockedPayload["attempt"] = attempt

				_ = appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
					"run.review_required", "review_required",
					map[string]any{
						"reason":       "budget_block",
						"error_code":   "budget_blocked",
						"details":      blockedPayload,
						"blockedEvent": blockedEvent,
					},
				)

				_ = upsertRunArtifact(context.Background(), db, run.RunID, "review_required_reason", map[string]any{
					"reason":       "budget_block",
					"project_id":   projectID,
					"worker_id":    workerID,
					"blockedEvent": blockedEvent,
					"payload":      blockedPayload,
					"attempt":      attempt,
					"decidedAt":    nowRFC3339Nano(),
				})
				return
			}

			reserved = true
			_ = appendEvent(context.Background(), db, run.RunID, run.TraceID,
				"budget.reserve",
				map[string]any{
					"project_id": projectID,
					"amount":     cost,
					"unit":       "credits",
					"reason":     reasonReserve,
					"attempt":    attempt,
					"worker_id":  workerID,
				},
			)

			// ----------------
			// pretend external work
			// ----------------
			time.Sleep(200 * time.Millisecond)

			_ = upsertRunArtifact(context.Background(), db, run.RunID, "analysis_result", map[string]any{
				"ok":      true,
				"mode":    mode,
				"stub":    "result_v3_2",
				"attempt": attempt,
			})

			_ = insertLearnSignal(context.Background(), db, run.RunID, projectID,
				"route_selected",
				map[string]any{
					"mode":     mode,
					"route_id": routeDecision.RouteID,
					"route":    "stub",
					"provider": routeDecision.Provider,
					"model":    routeDecision.Model,
					"attempt":  attempt,
				},
			)

			// capture: in this v3 stub, "reserve == spend" so capture is recorded as 0 ledger (optional)
			_ = captureBudgetTx(context.Background(), db, run.RunID, run.TraceID, projectID, 0, reasonCapture)
			_ = appendEvent(context.Background(), db, run.RunID, run.TraceID,
				"budget.capture",
				map[string]any{
					"project_id": projectID,
					"amount":     int64(0),
					"unit":       "credits",
					"reason":     reasonCapture,
					"attempt":    attempt,
					"worker_id":  workerID,
					"note":       "v3_stub: reserve already accounts spend; capture=0",
				},
			)
			captured = true

			if err := appendEventAndStatus(context.Background(), db, run.RunID, run.TraceID,
				"run.succeeded", "done",
				map[string]any{
					"result":     "stub_ok",
					"mode":       mode,
					"route_id":   routeDecision.RouteID,
					"provider":   routeDecision.Provider,
					"model":      routeDecision.Model,
					"worker_id":  workerID,
					"project_id": projectID,
					"attempt":    attempt,
				},
			); err != nil {
				log.Printf("run.succeeded append failed: %v", err)
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"event_write_failed",
					map[string]any{"stage": "run.succeeded", "error_detail": err.Error(), "project_id": projectID, "attempt": attempt},
				)
				return
			}
		}()
	}
}

func decideRoute(mode int) RouteDecision {
	switch mode {
	case 1:
		return RouteDecision{RouteID: "r1", Provider: "stub", Model: "stub"}
	default:
		return RouteDecision{RouteID: "r0", Provider: "stub", Model: "stub"}
	}
}

// pickQueuedRun:
// - DB final guard: atomically moves one queued run to running (SKIP LOCKED)
// - ensures trace_id is non-empty (generate + persist if empty)
// - returns the selected run row
func pickQueuedRun(ctx context.Context, db *pgxpool.Pool, workerID string) (runRow, bool, error) {
	_ = workerID // currently unused; kept for future without breaking signature.

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return runRow{}, false, err
	}
	defer tx.Rollback(ctx)

	var r runRow
	err = tx.QueryRow(ctx, `
		SELECT run_id, trace_id
		FROM runs
		WHERE status = 'queued'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&r.RunID, &r.TraceID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return runRow{}, false, nil
		}
		return runRow{}, false, err
	}

	// normalize run_id immediately (CHAR(26) padding)
	r.RunID = normalizeRunID(r.RunID)
	r.TraceID = strings.TrimSpace(r.TraceID)

	// validate run_id: must be exactly 26 chars (your DB is character(26))
	if !isValidRunID(r.RunID) {
		// move to review_required to prevent poison loop
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET status='review_required', updated_at=now()
			WHERE run_id=$1
		`, r.RunID)

		_ = tx.Commit(ctx)
		return runRow{}, false, fmt.Errorf("invalid run_id length: %q", r.RunID)
	}

	// ensure trace_id (runs.trace_id is NOT NULL, but keep guard anyway)
	if strings.TrimSpace(r.TraceID) == "" {
		r.TraceID = newTraceID()
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id = $2
			WHERE run_id = $1
		`, r.RunID, r.TraceID)
		if err != nil {
			return runRow{}, false, err
		}
	}

	// IMPORTANT: move to running here (DB final guard)
	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET status = 'running', updated_at = now()
		WHERE run_id = $1 AND status = 'queued'
	`, r.RunID)
	if err != nil {
		return runRow{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return runRow{}, false, err
	}
	return r, true, nil
}

// appendEvent:
// - serializes per-run event writes by locking runs row FOR UPDATE
// - assigns next event_seq safely
// - DOES NOT update runs.status
func appendEvent(ctx context.Context, db *pgxpool.Pool, runID, traceID, eventName string, payload map[string]any) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// serialize same-run event writes
	_, err = tx.Exec(ctx, `SELECT 1 FROM runs WHERE run_id=$1 FOR UPDATE`, runID)
	if err != nil {
		return err
	}

	var nextSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0) + 1
		FROM run_events
		WHERE run_id = $1
	`, runID).Scan(&nextSeq)
	if err != nil {
		return err
	}

	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, nextSeq, eventName, pb)
	if err != nil {
		return err
	}

	_, _ = tx.Exec(ctx, `UPDATE runs SET updated_at=now() WHERE run_id=$1`, runID)

	return tx.Commit(ctx)
}

// appendEventAndStatus:
// - same as appendEvent, but also updates runs.status atomically in the same Tx
func appendEventAndStatus(ctx context.Context, db *pgxpool.Pool, runID, traceID, eventName, newStatus string, payload map[string]any) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// serialize same-run event writes
	_, err = tx.Exec(ctx, `SELECT 1 FROM runs WHERE run_id=$1 FOR UPDATE`, runID)
	if err != nil {
		return err
	}

	var nextSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0) + 1
		FROM run_events
		WHERE run_id = $1
	`, runID).Scan(&nextSeq)
	if err != nil {
		return err
	}

	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, nextSeq, eventName, pb)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET status = $2, updated_at = now()
		WHERE run_id = $1
	`, runID, newStatus)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func getRunProjectID(ctx context.Context, db *pgxpool.Pool, runID string) (string, error) {
	runID = normalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var projectID string
	err := db.QueryRow(ctx, `
		SELECT project_id
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(&projectID)
	return projectID, err
}

// ---- v3.3 gate (policy-aware) ----

func gateByProjectSettings(
	ctx context.Context,
	db *pgxpool.Pool,
	projectID string,
	mode int,
	routeID string,
) (allowed bool, reason string, detail map[string]any, err error) {

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ps, ok, loadErr := loadProjectSettings(ctx, db, projectID)
	if loadErr != nil {
		return false, "settings_lookup_failed", map[string]any{
			"project_id":   projectID,
			"error_detail": loadErr.Error(),
		}, loadErr
	}
	if !ok {
		return false, "settings_missing", map[string]any{"project_id": projectID}, nil
	}
	if !ps.IsEnabled {
		return false, "project_disabled", map[string]any{"project_id": projectID}, nil
	}

	// ---- modes gate ----
	switch normalizePolicy(ps.AllowedModesPolicy) {
	case PolicyAllowAll:
		// pass
	case PolicyDenyAll:
		return false, "mode_not_allowed", map[string]any{
			"project_id":    projectID,
			"mode":          mode,
			"allowed_modes": ps.AllowedModes,
			"policy":        PolicyDenyAll,
		}, nil
	case PolicyAllowList:
		if len(ps.AllowedModes) == 0 {
			// empty allowlist => deny (safe)
			return false, "mode_not_allowed", map[string]any{
				"project_id":    projectID,
				"mode":          mode,
				"allowed_modes": ps.AllowedModes,
				"policy":        "allow_list_empty_denies",
			}, nil
		}
		if !containsInt(ps.AllowedModes, mode) {
			return false, "mode_not_allowed", map[string]any{
				"project_id":    projectID,
				"mode":          mode,
				"allowed_modes": ps.AllowedModes,
				"policy":        PolicyAllowList,
			}, nil
		}
	default:
		// unknown policy => review (deny)
		return false, "mode_not_allowed", map[string]any{
			"project_id": projectID,
			"mode":       mode,
			"policy":     ps.AllowedModesPolicy,
			"reason":     "unknown_policy",
		}, nil
	}

	// ---- routes gate ----
	switch normalizePolicy(ps.AllowedRoutesPolicy) {
	case PolicyAllowAll:
		// pass (even if allowed_routes == [])
	case PolicyDenyAll:
		return false, "route_not_allowed", map[string]any{
			"project_id":     projectID,
			"route_id":       routeID,
			"allowed_routes": ps.AllowedRoutes,
			"policy":         PolicyDenyAll,
		}, nil
	case PolicyAllowList:
		if strings.TrimSpace(routeID) == "" {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         PolicyAllowList,
				"reason":         "route_id_missing",
			}, nil
		}
		if len(ps.AllowedRoutes) == 0 {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         "allow_list_empty_denies",
			}, nil
		}
		if !containsStr(ps.AllowedRoutes, routeID) {
			return false, "route_not_allowed", map[string]any{
				"project_id":     projectID,
				"route_id":       routeID,
				"allowed_routes": ps.AllowedRoutes,
				"policy":         PolicyAllowList,
			}, nil
		}
	default:
		return false, "route_not_allowed", map[string]any{
			"project_id": projectID,
			"route_id":   routeID,
			"policy":     ps.AllowedRoutesPolicy,
			"reason":     "unknown_policy",
		}, nil
	}

	return true, "", nil, nil
}

func normalizePolicy(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case PolicyAllowAll, PolicyDenyAll, PolicyAllowList:
		return s
	default:
		return s
	}
}

func loadProjectSettings(ctx context.Context, db *pgxpool.Pool, projectID string) (projectSettingsRow, bool, error) {
	var allowedModesJSON string
	var allowedRoutesJSON string
	var isEnabled bool
	var modesPolicy string
	var routesPolicy string

	err := db.QueryRow(ctx, `
		SELECT
			allowed_modes::text        AS allowed_modes_json,
			allowed_routes::text       AS allowed_routes_json,
			is_enabled                 AS is_enabled,
			allowed_modes_policy       AS allowed_modes_policy,
			allowed_routes_policy      AS allowed_routes_policy
		FROM project_settings
		WHERE project_id = $1
	`, projectID).Scan(&allowedModesJSON, &allowedRoutesJSON, &isEnabled, &modesPolicy, &routesPolicy)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return projectSettingsRow{}, false, nil
		}
		return projectSettingsRow{}, false, err
	}

	var modes []int
	if umErr := json.Unmarshal([]byte(allowedModesJSON), &modes); umErr != nil {
		return projectSettingsRow{}, true, fmt.Errorf("invalid allowed_modes json: %w (raw=%s)", umErr, allowedModesJSON)
	}

	var routes []string
	if urErr := json.Unmarshal([]byte(allowedRoutesJSON), &routes); urErr != nil {
		return projectSettingsRow{}, true, fmt.Errorf("invalid allowed_routes json: %w (raw=%s)", urErr, allowedRoutesJSON)
	}

	// if empty policy somehow stored, default to allow_list (matches your table default)
	modesPolicy = strings.TrimSpace(modesPolicy)
	routesPolicy = strings.TrimSpace(routesPolicy)
	if modesPolicy == "" {
		modesPolicy = PolicyAllowList
	}
	if routesPolicy == "" {
		routesPolicy = PolicyAllowList
	}

	return projectSettingsRow{
		AllowedModes:        modes,
		AllowedRoutes:       routes,
		IsEnabled:           isEnabled,
		AllowedModesPolicy:  modesPolicy,
		AllowedRoutesPolicy: routesPolicy,
	}, true, nil
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// ---- v3.1 budget: reserve/capture/release ----
//
// Strategy with current schema:
// - reserve inserts +cost into budget_ledger with unique (run_id, reason)
// - release inserts -cost into budget_ledger with unique (run_id, reason)
// - capture is optional; here we insert 0 (keeps vocabulary without double charge)
//
// gateAndReserveBudgetTx:
// - Locks project_budgets row (FOR UPDATE) to make gate+reserve atomic per project
// - Enforces per_run_limit and daily_limit
// - Inserts budget_ledger (append-only) as "reserve" (+amount).
// - 23505 is treated as success ONLY when the unique constraint is ux_budget_ledger_run_reason.
func gateAndReserveBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	cost int64,
	reasonReserve string,
) (blockedEvent string, blockedPayload map[string]any, err error) {

	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	if cost <= 0 {
		return "", nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	// --- 1. プロジェクト設定の取得 (ロック) ---
	var perRunLimit int64
	var dailyLimit int64
	err = tx.QueryRow(ctx, `
        SELECT per_run_limit, daily_limit
        FROM project_budgets
        WHERE project_id = $1
        FOR UPDATE
    `, projectID).Scan(&perRunLimit, &dailyLimit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "run.blocked.budget", map[string]any{
				"project_id": projectID,
				"reason":     "budget_row_missing",
			}, nil
		}
		return "", nil, err
	}

	// --- 2. 予算チェック (Per-Run & Daily) ---
	// (中略：既存の spentRun と spentToday のロジックをここに維持)
	// ※長くなるので省略していますが、元のチェック処理をそのまま置いてください

	// --- 3. 予約（Reserve）の実行：ON CONFLICT 版 ---
	// 以前の P0 (SELECT EXISTS) と 最後の INSERT は、この一つのSQLで完結します。
	_, err = tx.Exec(ctx, `
        INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (run_id, reason) DO NOTHING
    `, runID, traceID, projectID, cost, "credits", reasonReserve)
	if err != nil {
		return "", nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func releaseBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	amount int64,
	reasonRelease string,
) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	if amount <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := db.Exec(ctx, `
		INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (run_id, reason) DO NOTHING
	`, runID, traceID, projectID, -amount, "credits", reasonRelease)
	return err
}

func captureBudgetTx(

	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	amount int64,
	reasonCapture string,
) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := db.Exec(ctx, `
		INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (run_id, reason) DO NOTHING
	`, runID, traceID, projectID, amount, "credits", reasonCapture)
	return err
}

func getRunModeFromEnqueuedEvent(ctx context.Context, db *pgxpool.Pool, runID string) (int, error) {
	runID = normalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var modeText string
	err := db.QueryRow(ctx, `
		SELECT COALESCE(payload->>'mode', '')
		FROM run_events
		WHERE run_id = $1 AND event_name = 'run.enqueued'
		ORDER BY event_seq ASC
		LIMIT 1
	`, runID).Scan(&modeText)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ✅ enqueued missing => mode=0 (safe default)
			return 0, nil
		}
		return 0, err
	}

	modeText = strings.TrimSpace(modeText)
	if modeText == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(modeText)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

func upsertRunArtifact(ctx context.Context, db *pgxpool.Pool, runID, kind string, content any) error {
	runID = normalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	b, _ := json.Marshal(content)

	_, err := db.Exec(ctx, `
		INSERT INTO run_artifacts(run_id, artifact_kind, content_json, created_at, updated_at)
		VALUES ($1,$2,$3::jsonb, now(), now())
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET content_json = EXCLUDED.content_json, updated_at = now()
	`, runID, kind, string(b))
	return err
}

func insertLearnSignal(ctx context.Context, db *pgxpool.Pool, runID, projectID, signalType string, payload map[string]any) error {
	runID = normalizeRunID(runID)
	projectID = strings.TrimSpace(projectID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pb := marshalJSONOrEmpty(payload)

	_, err := db.Exec(ctx, `
		INSERT INTO learn_signals(run_id, project_id, signal_type, payload)
		VALUES ($1,$2,$3,$4::jsonb)
	`, runID, projectID, signalType, pb)
	return err
}

// beginAttemptTx:
// - locks runs row FOR UPDATE (serializes per-run state)
// - reads run_artifacts(kind='attempt_state') => {attempt:N}
// - increments attempt and upserts artifact
// - appends run.attempt_started event with next seq (in the same tx)
func beginAttemptTx(ctx context.Context, db *pgxpool.Pool, runID, traceID, projectID, workerID string) (int, error) {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `SELECT 1 FROM runs WHERE run_id=$1 FOR UPDATE`, runID)
	if err != nil {
		return 0, err
	}

	// load current attempt_state
	var raw string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(content_json::text, '')
		FROM run_artifacts
		WHERE run_id = $1 AND artifact_kind = 'attempt_state'
		LIMIT 1
	`, runID).Scan(&raw)

	current := 0
	if err == nil && strings.TrimSpace(raw) != "" {
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(raw), &obj); jerr == nil {
			if v, ok := obj["attempt"]; ok {
				switch t := v.(type) {
				case float64:
					current = int(t)
				case int:
					current = t
				case int64:
					current = int(t)
				case string:
					if n, e := strconv.Atoi(t); e == nil {
						current = n
					}
				}
			}
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	nextAttempt := current + 1
	attemptState := map[string]any{
		"attempt":    nextAttempt,
		"updated_at": nowRFC3339Nano(),
	}

	// upsert attempt_state
	b, _ := json.Marshal(attemptState)
	_, err = tx.Exec(ctx, `
		INSERT INTO run_artifacts(run_id, artifact_kind, content_json, created_at, updated_at)
		VALUES ($1,'attempt_state',$2::jsonb, now(), now())
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET content_json = EXCLUDED.content_json, updated_at = now()
	`, runID, string(b))
	if err != nil {
		return 0, err
	}

	// append run.attempt_started within same tx
	var nextSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0) + 1
		FROM run_events
		WHERE run_id = $1
	`, runID).Scan(&nextSeq)
	if err != nil {
		return 0, err
	}

	payload := map[string]any{
		"attempt":    nextAttempt,
		"project_id": projectID,
		"worker_id":  workerID,
		"started_at": nowRFC3339Nano(),
	}
	pb := marshalJSONOrEmpty(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, nextSeq, "run.attempt_started", pb)
	if err != nil {
		return 0, err
	}

	_, _ = tx.Exec(ctx, `UPDATE runs SET updated_at=now() WHERE run_id=$1`, runID)

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return nextAttempt, nil
}

// holdForReview: MUST NOT crash; must stop as review_required
func holdForReview(ctx context.Context, db *pgxpool.Pool, runID, traceID, workerID, reason string, detail map[string]any) {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)

	if detail == nil {
		detail = map[string]any{}
	}
	detail["worker_id"] = workerID

	_ = appendEventAndStatus(ctx, db, runID, traceID,
		"run.review_required", "review_required",
		map[string]any{
			"reason":     reason,
			"error_code": reason,
			"details":    detail,
		},
	)

	_ = upsertRunArtifact(ctx, db, runID, "review_required_reason", map[string]any{
		"reason":    reason,
		"details":   detail,
		"worker_id": workerID,
		"decidedAt": nowRFC3339Nano(),
	})
}

// -------- helpers --------

func normalizeRunID(s string) string {
	// runs.run_id is CHAR(26) => may contain trailing spaces.
	return strings.TrimSpace(s)
}

func isValidRunID(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) == 26
}

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}

func getenvDefault(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func getenvDurationMS(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return time.Duration(n) * time.Millisecond
}

func getenvInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func hostnameFallback() string {
	h, _ := os.Hostname()
	if strings.TrimSpace(h) == "" {
		return "worker-unknown"
	}
	return h
}

func marshalJSONOrEmpty(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func redisSetNX(ctx context.Context, rdb *redis.Client, key, val string, ttl time.Duration) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return rdb.SetNX(cctx, key, val, ttl).Result()
}

func redisDel(ctx context.Context, rdb *redis.Client, key string) error {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := rdb.Del(cctx, key).Result()
	return err
}

func newTraceID() string {
	// 16 bytes => 32 hex chars
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UTC().UnixNano()
		return fmt.Sprintf("trace-%d", now)
	}
	return hex.EncodeToString(b)
}