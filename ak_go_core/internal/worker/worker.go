package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/ak_go_core/internal/infra/redis"
)

type Config struct {
	WorkerID string
	Poll     time.Duration
	Cost     int64
}

type Worker struct {
	store *Store
	rdb   *redis.Client
	cfg   Config

	logger *log.Logger
}

func NewWorker(store *Store, rdb *redis.Client, cfg Config, logger *log.Logger) *Worker {
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Worker{store: store, rdb: rdb, cfg: cfg, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Printf("AK Go Worker started. worker_id=%s poll=%s cost=%d", w.cfg.WorkerID, w.cfg.Poll, w.cfg.Cost)

	ticker := time.NewTicker(w.cfg.Poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.processOnce(ctx)
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) {
	pickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	run, ok, pickErr := w.store.PickQueuedRun(pickCtx, w.cfg.WorkerID)
	cancel()

	if pickErr != nil {
		w.logger.Printf("PickQueuedRun error: %v", pickErr)
		return
	}
	if !ok {
		return
	}

	run.RunID = NormalizeRunID(run.RunID)
	run.TraceID = strings.TrimSpace(run.TraceID)

	if !IsValidRunID(run.RunID) {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "invalid_run_id",
			map[string]any{"run_id": run.RunID, "expected_len": 26},
		)
		return
	}

	lockKey := "run:" + run.RunID + ":lock"
	locked, err := redisx.SetNX(context.Background(), w.rdb, lockKey, "1", 90*time.Second)
	if err != nil {
		w.logger.Printf("redis SetNX error: %v", err)
		return
	}
	if !locked {
		return
	}
	defer func() { _ = redisx.Del(context.Background(), w.rdb, lockKey) }()

	st, stErr := w.store.GetRunState(context.Background(), run.RunID)
	if stErr != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "state_lookup_failed",
			map[string]any{"error_detail": stErr.Error()},
		)
		return
	}
	if st != "running" {
		return
	}

	projectID, err := w.store.GetRunProjectID(context.Background(), run.RunID)
	if err != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "project_id_lookup_failed",
			map[string]any{"error_detail": err.Error()},
		)
		return
	}

	pickedAt := time.Now().UTC().Format(time.RFC3339Nano)

	if err := w.store.AppendEvent(context.Background(), run.RunID, run.TraceID,
		"run.running",
		map[string]any{
			"worker_id":  w.cfg.WorkerID,
			"picked_at":  pickedAt,
			"project_id": projectID,
		},
	); err != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "event_write_failed",
			map[string]any{"stage": "run.running", "error_detail": err.Error(), "project_id": projectID},
		)
		return
	}

	attempt, aErr := w.store.BeginAttemptTx(context.Background(), run.RunID, run.TraceID, projectID, w.cfg.WorkerID)
	if aErr != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "attempt_begin_failed",
			map[string]any{"error_detail": aErr.Error(), "project_id": projectID},
		)
		return
	}

	mode, modeErr := w.store.GetRunModeFromEnqueuedEvent(context.Background(), run.RunID)
	if modeErr != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "mode_lookup_failed",
			map[string]any{"error_detail": modeErr.Error(), "project_id": projectID},
		)
		return
	}

	routeDecision := DecideRoute(mode)

	allowed, gateReason, gateDetail, gateErr := GateByProjectSettings(
		context.Background(), w.store.db,
		projectID, mode, routeDecision.RouteID,
	)
	if gateErr != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "project_gate_error",
			map[string]any{"project_id": projectID, "mode": mode, "route_id": routeDecision.RouteID, "error_detail": gateErr.Error()},
		)
		return
	}
	if !allowed {
		_ = w.store.AppendEventAndStatus(context.Background(), run.RunID, run.TraceID,
			"run.review_required", "review_required",
			map[string]any{
				"reason":     gateReason,
				"error_code": "project_gate_denied",
				"details":    gateDetail,
				"worker_id":  w.cfg.WorkerID,
				"project_id": projectID,
				"mode":       mode,
				"route_id":   routeDecision.RouteID,
				"attempt":    attempt,
			},
		)
		_ = w.store.UpsertRunArtifact(context.Background(), run.RunID, run.TraceID, "review_required_reason",
			map[string]any{
				"reason":     gateReason,
				"project_id": projectID,
				"mode":       mode,
				"route_id":   routeDecision.RouteID,
				"details":    gateDetail,
				"worker_id":  w.cfg.WorkerID,
				"attempt":    attempt,
				"decidedAt":  NowRFC3339Nano(),
			},
		)
		_ = w.store.InsertLearnSignal(context.Background(), run.RunID, projectID, "project_gate_denied",
			map[string]any{"mode": mode, "route_id": routeDecision.RouteID, "reason": gateReason, "details": gateDetail, "worker_id": w.cfg.WorkerID, "attempt": attempt},
		)
		return
	}

	_ = w.store.InsertLearnSignal(context.Background(), run.RunID, projectID, "mode_received",
		map[string]any{"mode": mode, "worker_id": w.cfg.WorkerID, "attempt": attempt},
	)

	_ = w.store.UpsertRunArtifact(context.Background(), run.RunID, run.TraceID, "route_decision",
		map[string]any{
			"mode":      mode,
			"route_id":  routeDecision.RouteID,
			"provider":  routeDecision.Provider,
			"model":     routeDecision.Model,
			"attempt":   attempt,
			"decidedAt": NowRFC3339Nano(),
		},
	)

	// ---- v3.1 budget reserve/capture/release (attempt-aware) ----
	reserved := false
	captured := false
	reasonReserve := fmt.Sprintf("reserve_run_cost_a%d", attempt)
	reasonRelease := fmt.Sprintf("release_run_cost_a%d", attempt)
	reasonCapture := fmt.Sprintf("capture_run_cost_a%d", attempt)

	defer func() {
		if !reserved || captured {
			return
		}
		_ = ReleaseBudgetTx(context.Background(), w.store.db, run.RunID, run.TraceID, projectID, w.cfg.Cost, reasonRelease)
		_ = w.store.AppendEvent(context.Background(), run.RunID, run.TraceID, "budget.release",
			map[string]any{"project_id": projectID, "amount": w.cfg.Cost, "unit": "credits", "reason": reasonRelease, "attempt": attempt, "worker_id": w.cfg.WorkerID},
		)
	}()

	blockedEvent, blockedPayload, err := GateAndReserveBudgetTx(context.Background(), w.store.db, run.RunID, run.TraceID, projectID, w.cfg.Cost, reasonReserve)
	if err != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "budget_gate_reserve_failed",
			map[string]any{"error_detail": err.Error(), "project_id": projectID, "attempt": attempt},
		)
		return
	}
	if blockedEvent != "" {
		if blockedPayload == nil {
			blockedPayload = map[string]any{}
		}
		blockedPayload["worker_id"] = w.cfg.WorkerID
		blockedPayload["project_id"] = projectID
		blockedPayload["blocked_event"] = blockedEvent
		blockedPayload["attempt"] = attempt

		_ = w.store.AppendEventAndStatus(context.Background(), run.RunID, run.TraceID,
			"run.review_required", "review_required",
			map[string]any{"reason": "budget_block", "error_code": "budget_blocked", "details": blockedPayload, "blockedEvent": blockedEvent},
		)
		_ = w.store.UpsertRunArtifact(context.Background(), run.RunID, run.TraceID, "review_required_reason",
			map[string]any{"reason": "budget_block", "project_id": projectID, "worker_id": w.cfg.WorkerID, "blockedEvent": blockedEvent, "payload": blockedPayload, "attempt": attempt, "decidedAt": NowRFC3339Nano()},
		)
		return
	}

	reserved = true
	_ = w.store.AppendEvent(context.Background(), run.RunID, run.TraceID, "budget.reserve",
		map[string]any{"project_id": projectID, "amount": w.cfg.Cost, "unit": "credits", "reason": reasonReserve, "attempt": attempt, "worker_id": w.cfg.WorkerID},
	)

	// ---- stub external work ----
	time.Sleep(200 * time.Millisecond)

	_ = w.store.UpsertRunArtifact(context.Background(), run.RunID, run.TraceID, "analysis_result",
		map[string]any{"ok": true, "mode": mode, "stub": "result_v3_2", "attempt": attempt},
	)

	_ = w.store.InsertLearnSignal(context.Background(), run.RunID, projectID, "route_selected",
		map[string]any{"mode": mode, "route_id": routeDecision.RouteID, "provider": routeDecision.Provider, "model": routeDecision.Model, "attempt": attempt},
	)

	_ = CaptureBudgetTx(context.Background(), w.store.db, run.RunID, run.TraceID, projectID, 0, reasonCapture)
	_ = w.store.AppendEvent(context.Background(), run.RunID, run.TraceID, "budget.capture",
		map[string]any{"project_id": projectID, "amount": int64(0), "unit": "credits", "reason": reasonCapture, "attempt": attempt, "worker_id": w.cfg.WorkerID, "note": "v3_stub: reserve already accounts spend; capture=0"},
	)
	captured = true

	if err := w.store.AppendEventAndStatus(context.Background(), run.RunID, run.TraceID,
		"run.succeeded", "done",
		map[string]any{
			"result":     "stub_ok",
			"mode":       mode,
			"route_id":   routeDecision.RouteID,
			"provider":   routeDecision.Provider,
			"model":      routeDecision.Model,
			"worker_id":  w.cfg.WorkerID,
			"project_id": projectID,
			"attempt":    attempt,
		},
	); err != nil {
		w.holdForReview(context.Background(), run.RunID, run.TraceID, "event_write_failed",
			map[string]any{"stage": "run.succeeded", "error_detail": err.Error(), "project_id": projectID, "attempt": attempt},
		)
		return
	}
}

func (w *Worker) holdForReview(ctx context.Context, runID, traceID, reason string, detail map[string]any) {
	runID = NormalizeRunID(runID)
	traceID = strings.TrimSpace(traceID)
	if detail == nil {
		detail = map[string]any{}
	}
	detail["worker_id"] = w.cfg.WorkerID

	_ = w.store.AppendEventAndStatus(ctx, runID, traceID,
		"run.review_required", "review_required",
		map[string]any{"reason": reason, "error_code": reason, "details": detail},
	)

	_ = w.store.UpsertRunArtifact(ctx, runID, traceID, "review_required_reason",
		map[string]any{"reason": reason, "details": detail, "worker_id": w.cfg.WorkerID, "decidedAt": NowRFC3339Nano()},
	)
}