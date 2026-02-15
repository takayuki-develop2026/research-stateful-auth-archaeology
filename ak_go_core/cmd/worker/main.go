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
	"github.com/jackc/pgx/v5/pgconn"
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

// ------------------------
// RunArtifacts contract (DDL-aligned)
// ------------------------
//
// NOTE: Your run_artifacts has ck_run_artifacts_schema_version_v1:
//   CHECK (schema_version = '1.0')
//
// Even if the column default shows 'v1.0', the check constraint wins.
// We MUST write '1.0'.
const (
	RunArtifactSchemaVersion = "1.0"
)

// ------------------------
// main
// ------------------------

func main() {
	dsn := mustEnv("AK_DB_DSN")

	// NOTE: compose has AK_WORKER_NAME but code uses AK_WORKER_ID.
	// Keep current behavior but ensure uniqueness for observability.
	rawID := strings.TrimSpace(os.Getenv("AK_WORKER_ID"))
	if rawID == "" {
		rawID = strings.TrimSpace(os.Getenv("AK_WORKER_NAME"))
	}
	host := hostnameFallback()
	workerID := rawID
	if workerID == "" {
		workerID = host
	}
	// Make it unique even if compose accidentally duplicates AK_WORKER_ID.
	if !strings.Contains(workerID, "@") {
		workerID = fmt.Sprintf("%s@%s", workerID, host)
	}

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	redisAddr := getenvDefault("AK_REDIS_ADDR", "ak_redis:6379")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer func() { _ = rdb.Close() }()

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

			// Final sanity: if someone changed the run state after we picked it, stop.
			st, stErr := getRunState(context.Background(), db, run.RunID)
			if stErr != nil {
				holdForReview(context.Background(), db, run.RunID, run.TraceID, workerID,
					"state_lookup_failed",
					map[string]any{"error_detail": stErr.Error()},
				)
				return
			}
			if st != "running" {
				// Do not write additional events if not running.
				return
			}

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

			// run.running is observability ONLY (state already set to 'running' by pickQueuedRun)
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

				_ = upsertRunArtifact(context.Background(), db, run.RunID, run.TraceID, "review_required_reason", map[string]any{
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

			_ = upsertRunArtifact(context.Background(), db, run.RunID, run.TraceID, "route_decision", map[string]any{
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

				_ = upsertRunArtifact(context.Background(), db, run.RunID, run.TraceID, "review_required_reason", map[string]any{
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

			_ = upsertRunArtifact(context.Background(), db, run.RunID, run.TraceID, "analysis_result", map[string]any{
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

		// next loop
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
// - sets state to running
//
// NOTE(P0): runs.status is "public 2-value or empty" in API layer.
//           So DB should NOT store "running" into status. Keep status empty except:
//           - review_required
//           - failed
func pickQueuedRun(ctx context.Context, db *pgxpool.Pool, workerID string) (runRow, bool, error) {
	_ = workerID // reserved for future: picked_by column, etc.

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
		WHERE state = 'queued'
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

	r.RunID = normalizeRunID(r.RunID)
	r.TraceID = strings.TrimSpace(r.TraceID)

	if !isValidRunID(r.RunID) {
		// move to review_required safely; do not proceed
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET state='review_required', status='review_required', result=COALESCE(NULLIF(result,''),'pending'), updated_at=now()
			WHERE run_id=$1
		`, r.RunID)
		_ = tx.Commit(ctx)
		return runRow{}, false, fmt.Errorf("invalid run_id length: %q", r.RunID)
	}

	// Ensure trace_id exists (Tx-safe).
	// NOTE: runs.trace_id is NOT NULL, but might be '' depending on insert path.
	if r.TraceID == "" {
		r.TraceID = newTraceID()
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id=$2
			WHERE run_id=$1
		`, r.RunID, r.TraceID)
		if err != nil {
			return runRow{}, false, err
		}
	}

	// State transition must affect exactly 1 row.
	tag, err := tx.Exec(ctx, `
		UPDATE runs
		SET state='running', status=NULL, result=COALESCE(NULLIF(result,''),'pending'), updated_at=now()
		WHERE run_id=$1 AND state='queued'
	`, r.RunID)
	if err != nil {
		return runRow{}, false, err
	}
	if tag.RowsAffected() != 1 {
		// someone else took it (should not happen due to lock, but keep safe)
		return runRow{}, false, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return runRow{}, false, err
	}
	return r, true, nil
}

// appendEvent:
// - locks runs row FOR UPDATE
// - uses runs.next_event_seq as SoT
// - increments runs.next_event_seq in the same Tx
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

	seq, err := nextEventSeqTx(ctx, tx, runID)
	if err != nil {
		return err
	}

	// Ensure trace_id exists in runs too (SoT alignment).
	if traceID != "" {
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id = CASE WHEN COALESCE(NULLIF(BTRIM(trace_id),''),'') = '' THEN $2 ELSE trace_id END
			WHERE run_id=$1
		`, runID, traceID)
	}

	pb := marshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, seq, eventName, pb)
	if err != nil {
		return err
	}

	// advance next_event_seq and touch updated_at
	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET next_event_seq=$2, updated_at=now()
		WHERE run_id=$1
	`, runID, seq+1)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// appendEventAndStatus:
// - same as appendEvent, but also updates runs.state/status/result
//
// P0 policy:
// - state is internal progress (queued/running/done/review_required/failed/blocked...)
// - status is public 2-value: review_required/failed or NULL (empty)
// - result is pending/success/failed
func appendEventAndStatus(ctx context.Context, db *pgxpool.Pool, runID, traceID, eventName, newState string, payload map[string]any) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	seq, err := nextEventSeqTx(ctx, tx, runID)
	if err != nil {
		return err
	}

	if traceID != "" {
		_, _ = tx.Exec(ctx, `
			UPDATE runs
			SET trace_id = CASE WHEN COALESCE(NULLIF(BTRIM(trace_id),''),'') = '' THEN $2 ELSE trace_id END
			WHERE run_id=$1
		`, runID, traceID)
	}

	pb := marshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, seq, eventName, pb)
	if err != nil {
		return err
	}

	// derive status/result from newState (DB-level safety)
	var status any = nil
	var result any = nil

	switch newState {
	case "done":
		status = nil
		result = "success"
	case "failed":
		status = "failed"
		result = "failed"
	case "review_required":
		status = "review_required"
		result = "pending"
	default:
		// queued/running/blocked/...
		status = nil
		// keep existing or pending
		result = nil
	}

	var tag pgconn.CommandTag
	if result == nil {
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET state=$2,
			    status=$3,
			    next_event_seq=$4,
			    updated_at=now()
			WHERE run_id=$1
		`, runID, newState, status, seq+1)
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE runs
			SET state=$2,
			    status=$3,
			    result=$4,
			    next_event_seq=$5,
			    updated_at=now()
			WHERE run_id=$1
		`, runID, newState, status, result, seq+1)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("runs update affected=%d (run_id=%s)", tag.RowsAffected(), runID)
	}

	return tx.Commit(ctx)
}

// nextEventSeqTx:
// - locks runs row
// - reads next_event_seq (default 1 if null/0)
// - reads max(event_seq) for safety
// - repairs runs.next_event_seq if needed
func nextEventSeqTx(ctx context.Context, tx pgx.Tx, runID string) (int64, error) {
	runID = normalizeRunID(runID)

	// 1) lock runs row
	var next int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(next_event_seq,0), 1)
		FROM runs
		WHERE run_id=$1
		FOR UPDATE
	`, runID).Scan(&next)
	if err != nil {
		return 0, err
	}

	// 2) read max(event_seq) (same tx, safe enough because runs row is locked)
	var maxSeq int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), 0)
		FROM run_events
		WHERE run_id=$1
	`, runID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}

	// 3) choose the safe seq
	safe := next
	if maxSeq+1 > safe {
		safe = maxSeq + 1
	}

	// 4) repair if needed
	if safe != next {
		_, err = tx.Exec(ctx, `
			UPDATE runs
			SET next_event_seq = $2,
			    updated_at = now()
			WHERE run_id = $1
		`, runID, safe)
		if err != nil {
			return 0, err
		}
	}

	return safe, nil
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

func getRunState(ctx context.Context, db *pgxpool.Pool, runID string) (string, error) {
	runID = normalizeRunID(runID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var st string
	err := db.QueryRow(ctx, `
		SELECT state
		FROM runs
		WHERE run_id=$1
	`, runID).Scan(&st)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(st), nil
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
		// unknown policy => deny (safe)
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
		// pass
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
	return strings.TrimSpace(strings.ToLower(s))
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

	// if empty policy somehow stored, default to allow_list (matches typical table default)
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

// ----------------------------------------------------------------------
// v3.1 budget: reserve/capture/release
// ----------------------------------------------------------------------

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

	// --- 1) load+lock project budget row ---
	var perRunLimit int64
	var dailyLimit int64
	var monthlyLimit int64
	err = tx.QueryRow(ctx, `
		SELECT per_run_limit, daily_limit, monthly_limit
		FROM project_budgets
		WHERE project_id = $1
		FOR UPDATE
	`, projectID).Scan(&perRunLimit, &dailyLimit, &monthlyLimit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "run.blocked.budget", map[string]any{
				"project_id": projectID,
				"reason":     "budget_row_missing",
			}, nil
		}
		return "", nil, err
	}

	// --- 2) per-run limit ---
	if perRunLimit > 0 && cost > perRunLimit {
		return "run.blocked.budget", map[string]any{
			"project_id":     projectID,
			"reason":         "per_run_limit_exceeded",
			"cost":           cost,
			"per_run_limit":  perRunLimit,
			"daily_limit":    dailyLimit,
			"monthly_limit":  monthlyLimit,
			"reserve_reason": reasonReserve,
		}, nil
	}

	// --- 3) daily limit gate ---
	var spentToday int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM budget_ledger
		WHERE project_id = $1
		  AND created_at >= date_trunc('day', now())
		  AND created_at <  date_trunc('day', now()) + interval '1 day'
	`, projectID).Scan(&spentToday)
	if err != nil {
		return "", nil, err
	}
	if dailyLimit > 0 && (spentToday+cost) > dailyLimit {
		return "run.blocked.budget", map[string]any{
			"project_id":     projectID,
			"reason":         "daily_limit_exceeded",
			"cost":           cost,
			"spent_today":    spentToday,
			"daily_limit":    dailyLimit,
			"monthly_limit":  monthlyLimit,
			"reserve_reason": reasonReserve,
		}, nil
	}

	// --- 4) monthly limit gate ---
	var spentThisMonth int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM budget_ledger
		WHERE project_id = $1
		  AND created_at >= date_trunc('month', now())
		  AND created_at <  date_trunc('month', now()) + interval '1 month'
	`, projectID).Scan(&spentThisMonth)
	if err != nil {
		return "", nil, err
	}
	if monthlyLimit > 0 && (spentThisMonth+cost) > monthlyLimit {
		return "run.blocked.budget", map[string]any{
			"project_id":       projectID,
			"reason":           "monthly_limit_exceeded",
			"cost":             cost,
			"spent_this_month": spentThisMonth,
			"monthly_limit":    monthlyLimit,
			"reserve_reason":   reasonReserve,
		}, nil
	}

	// --- 5) reserve ledger insert (idempotent by UNIQUE(run_id, reason)) ---
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
			// enqueued missing => mode=0 (safe default)
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
		// invalid => safe default
		return 0, nil
	}
	return n, nil
}

// ------------------------
// Run Artifacts (DDL-aligned UPSERT)
// ------------------------
//
// DDL requires NOT NULL:
// - schema_version must be '1.0'
// - trace_id non-empty
// - artifact_ref_kind = artifact_kind
// - artifact_ref_run_id = run_id::text
// - artifact_ref_trace_id = trace_id
// - trace_trace_id = trace_id
//
// Also checks:
// - artifact_kind must match regex
func upsertRunArtifact(ctx context.Context, db *pgxpool.Pool, runID, traceID, kind string, content any) error {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	kind = strings.TrimSpace(kind)

	if traceID == "" {
		// cannot satisfy NOT NULL + check constraints; treat as hard error
		return fmt.Errorf("trace_id is required for run_artifacts (run_id=%s kind=%s)", runID, kind)
	}
	if kind == "" {
		return fmt.Errorf("artifact_kind is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	b, err := json.Marshal(content)
	if err != nil {
		b = []byte(`{}`)
	}

	// IMPORTANT:
	// - run_artifacts.run_id is character(26) (bpchar)
	// - artifact_ref_run_id is text
	// If we reuse the same parameter ($1) for both bpchar and text in one statement,
	// Postgres can throw: "inconsistent types deduced for parameter $1 (42P08)".
	// So we pass run_id twice with separate placeholders.
	runIDText := runID

	_, err = db.Exec(ctx, `
		INSERT INTO run_artifacts(
			run_id,
			artifact_kind,
			content_json,
			created_at,
			updated_at,
			schema_version,
			trace_id,
			artifact_ref_kind,
			artifact_ref_run_id,
			artifact_ref_trace_id,
			trace_trace_id
		)
		VALUES (
			$1,
			$2,
			$3::jsonb,
			now(),
			now(),
			$4,
			$5,
			$2,
			$6,
			$5,
			$5
		)
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET
			content_json = EXCLUDED.content_json,
			updated_at  = now(),
			-- keep these aligned (defensive)
			schema_version        = EXCLUDED.schema_version,
			trace_id              = EXCLUDED.trace_id,
			artifact_ref_kind     = EXCLUDED.artifact_ref_kind,
			artifact_ref_run_id   = EXCLUDED.artifact_ref_run_id,
			artifact_ref_trace_id = EXCLUDED.artifact_ref_trace_id,
			trace_trace_id        = EXCLUDED.trace_trace_id
	`, runID, kind, string(b), RunArtifactSchemaVersion, traceID, runIDText)

	return err
}

func insertLearnSignal(ctx context.Context, db *pgxpool.Pool, runID, projectID, signalType string, payload map[string]any) error {
	runID = normalizeRunID(runID)
	projectID = strings.TrimSpace(projectID)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pb := marshalJSONOrEmptyMap(payload)

	_, err := db.Exec(ctx, `
		INSERT INTO learn_signals(run_id, project_id, signal_type, payload)
		VALUES ($1,$2,$3,$4::jsonb)
	`, runID, projectID, signalType, pb)
	return err
}

// beginAttemptTx:
// - locks runs row FOR UPDATE via nextEventSeqTx (serializes per-run state)
// - reads run_artifacts(kind='attempt_state') => {attempt:N}
// - increments attempt and upserts artifact (DDL-aligned)
// - appends run.attempt_started event using runs.next_event_seq (in the same tx)
func beginAttemptTx(ctx context.Context, db *pgxpool.Pool, runID, traceID, projectID, workerID string) (int, error) {
	runID = normalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	projectID = strings.TrimSpace(projectID)

	if traceID == "" {
		return 0, fmt.Errorf("trace_id is required for beginAttemptTx")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// lock + read seq
	seq, err := nextEventSeqTx(ctx, tx, runID)
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

	b, jerr := json.Marshal(attemptState)
	if jerr != nil {
		b = []byte(`{}`)
	}

	// IMPORTANT (same as upsertRunArtifact):
	// - run_id is bpchar(26)
	// - artifact_ref_run_id is text
	// Do NOT reuse the same parameter for both types in one SQL statement.
	runIDText := runID

	_, err = tx.Exec(ctx, `
		INSERT INTO run_artifacts(
			run_id,
			artifact_kind,
			content_json,
			created_at,
			updated_at,
			schema_version,
			trace_id,
			artifact_ref_kind,
			artifact_ref_run_id,
			artifact_ref_trace_id,
			trace_trace_id
		)
		VALUES (
			$1,
			'attempt_state',
			$2::jsonb,
			now(),
			now(),
			$3,
			$4,
			'attempt_state',
			$5,
			$4,
			$4
		)
		ON CONFLICT (run_id, artifact_kind)
		DO UPDATE SET
			content_json = EXCLUDED.content_json,
			updated_at  = now(),
			schema_version        = EXCLUDED.schema_version,
			trace_id              = EXCLUDED.trace_id,
			artifact_ref_kind     = EXCLUDED.artifact_ref_kind,
			artifact_ref_run_id   = EXCLUDED.artifact_ref_run_id,
			artifact_ref_trace_id = EXCLUDED.artifact_ref_trace_id,
			trace_trace_id        = EXCLUDED.trace_trace_id
	`, runID, string(b), RunArtifactSchemaVersion, traceID, runIDText)
	if err != nil {
		return 0, err
	}

	// append run.attempt_started within same tx
	payload := map[string]any{
		"attempt":    nextAttempt,
		"project_id": projectID,
		"worker_id":  workerID,
		"started_at": nowRFC3339Nano(),
	}
	pb := marshalJSONOrEmptyMap(payload)

	_, err = tx.Exec(ctx, `
		INSERT INTO run_events(run_id, trace_id, event_seq, event_name, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, runID, traceID, seq, "run.attempt_started", pb)
	if err != nil {
		return 0, err
	}

	// advance next_event_seq + touch updated_at
	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET next_event_seq=$2, updated_at=now()
		WHERE run_id=$1
	`, runID, seq+1)
	if err != nil {
		return 0, err
	}

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

	// best-effort; if trace_id empty it will return error, but MUST NOT crash
	_ = upsertRunArtifact(ctx, db, runID, traceID, "review_required_reason", map[string]any{
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

// marshalJSONOrEmptyMap returns a JSON string (not bytes) to use with ::jsonb
func marshalJSONOrEmptyMap(payload map[string]any) string {
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