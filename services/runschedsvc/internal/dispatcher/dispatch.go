package dispatcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"runschedsvc/postgres"
)

type DispatchConfig struct {
	ProjectID string
	LimitRuns int
	ActorType string
	ActorID   string
}

type Dispatcher struct {
	repo *postgres.RunSchedRepoV19
	cfg  DispatchConfig
}

func New(repo *postgres.RunSchedRepoV19, cfg DispatchConfig) *Dispatcher {
	if cfg.LimitRuns <= 0 {
		cfg.LimitRuns = 100
	}
	if strings.TrimSpace(cfg.ActorType) == "" {
		cfg.ActorType = "system"
	}
	if strings.TrimSpace(cfg.ActorID) == "" {
		cfg.ActorID = "runschedsvc"
	}
	return &Dispatcher{repo: repo, cfg: cfg}
}

func envBool(k string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	var n int64
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return def
	}
	return n
}

// DispatchOnce: claim queued scheduled_runs -> BudgetGate(v19) -> (PolicyGate later) -> create_run -> budget consume
func (d *Dispatcher) DispatchOnce(ctx context.Context) (created int, skipped int, err error) {
	claimed, err := d.repo.ClaimQueuedScheduledRuns(ctx, d.cfg.ProjectID, d.cfg.LimitRuns)
	if err != nil {
		return 0, 0, err
	}

	forceBudgetDeny := envBool("RUNSCHED_FORCE_BUDGET_DENY")
	forcePolicyDeny := envBool("RUNSCHED_FORCE_POLICY_DENY")

	// cost per run (credits). MVP: fixed 1.
	// You can override with RUNSCHED_RUN_COST=5 etc.
	runCost := envInt64("RUNSCHED_RUN_COST", 1)
	if runCost < 0 {
		runCost = 0
	}

	log.Printf("[runschedsvc][dispatch] claimed=%d force_budget_deny=%t force_policy_deny=%t run_cost=%d",
		len(claimed), forceBudgetDeny, forcePolicyDeny, runCost,
	)

	nowUTC := time.Now().UTC()
	var errorsCount int

	for _, c := range claimed {
		traceID := c.TraceID

		// -------------------------
		// Budget Gate (Phase2)
		// -------------------------
		if forceBudgetDeny {
			log.Printf("[runschedsvc][gate] budget deny (forced) scheduled_run_id=%d schedule_id=%d trace_id=%s",
				c.ScheduledRunID, c.ScheduleID, traceID)

			evID, evErr := d.repo.RegisterReasonEvidenceV19(
				ctx, d.cfg.ProjectID, traceID, d.cfg.ActorType, d.cfg.ActorID,
				"budget_cap_exceeded",
				"forced deny by RUNSCHED_FORCE_BUDGET_DENY",
				"runschedsvc:budgetdeny:"+d.cfg.ProjectID+":"+traceID,
			)
			if evErr != nil {
				log.Printf("[runschedsvc][gate] budget deny evidence FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, evErr)
				_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "budget_gate_evidence_failed", 0)
				errorsCount++
				continue
			}
			_ = d.repo.MarkSkipped(ctx, d.cfg.ProjectID, c.ScheduledRunID, "skipped_budget", "budget_cap_exceeded", evID)
			skipped++
			continue
		}

		allowed, reason, remDaily, remMonthly, gerr := d.repo.BudgetGateCheckV19(ctx, d.cfg.ProjectID, nowUTC, runCost)
		if gerr != nil {
			log.Printf("[runschedsvc][gate] budget check FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, gerr)
			_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "budget_gate_check_failed", 0)
			errorsCount++
			continue
		}

		if !allowed {
			msg := fmt.Sprintf("budget deny reason=%s cost=%d remaining_daily=%d remaining_monthly=%d", reason, runCost, remDaily, remMonthly)
			log.Printf("[runschedsvc][gate] budget deny scheduled_run_id=%d reason=%s", c.ScheduledRunID, msg)

			evID, evErr := d.repo.RegisterReasonEvidenceV19(
				ctx, d.cfg.ProjectID, traceID, d.cfg.ActorType, d.cfg.ActorID,
				reason, msg,
				"runschedsvc:budgetdeny:"+d.cfg.ProjectID+":"+traceID,
			)
			if evErr != nil {
				log.Printf("[runschedsvc][gate] budget deny evidence FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, evErr)
				_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "budget_gate_evidence_failed", 0)
				errorsCount++
				continue
			}
			_ = d.repo.MarkSkipped(ctx, d.cfg.ProjectID, c.ScheduledRunID, "skipped_budget", reason, evID)
			skipped++
			continue
		}

		// reserve before create_run (contract: gate before dispatch)
		reserveEvID, reserveEvErr := d.repo.RegisterReasonEvidenceV19(
			ctx, d.cfg.ProjectID, traceID, d.cfg.ActorType, d.cfg.ActorID,
			"budget_reserved",
			fmt.Sprintf("reserved cost=%d credits", runCost),
			"runschedsvc:budgetreserve:"+d.cfg.ProjectID+":"+traceID,
		)
		if reserveEvErr != nil {
			log.Printf("[runschedsvc][gate] budget reserve evidence FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, reserveEvErr)
			_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "budget_reserve_evidence_failed", 0)
			errorsCount++
			continue
		}
		if err := d.repo.BudgetReserveV19(ctx, d.cfg.ProjectID, c.ScheduledRunID, traceID, runCost, "budget_reserved", reserveEvID); err != nil {
			log.Printf("[runschedsvc][gate] budget reserve FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, err)
			_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "budget_reserve_failed", 0)
			errorsCount++
			continue
		}

		// -------------------------
		// Policy Gate (Phase1 keep forced deny only)
		// Phase2 policy ledger wiring comes next chat.
		// -------------------------
		if forcePolicyDeny {
			log.Printf("[runschedsvc][gate] policy deny (forced) scheduled_run_id=%d schedule_id=%d trace_id=%s",
				c.ScheduledRunID, c.ScheduleID, traceID)

			evID, evErr := d.repo.RegisterReasonEvidenceV19(
				ctx, d.cfg.ProjectID, traceID, d.cfg.ActorType, d.cfg.ActorID,
				"policy_default_deny",
				"forced deny by RUNSCHED_FORCE_POLICY_DENY",
				"runschedsvc:policydeny:"+d.cfg.ProjectID+":"+traceID,
			)
			if evErr != nil {
				log.Printf("[runschedsvc][gate] policy deny evidence FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, evErr)
				_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "policy_gate_evidence_failed", 0)
				errorsCount++
				continue
			}
			_ = d.repo.MarkSkipped(ctx, d.cfg.ProjectID, c.ScheduledRunID, "skipped_policy", "policy_default_deny", evID)
			skipped++
			continue
		}

		// -------------------------
		// OK -> create run
		// -------------------------
		res, err := d.repo.CreateRunForScheduled(ctx, d.cfg.ProjectID, c.ScheduledRunID, nowUTC)
		if err != nil {
			log.Printf("[runschedsvc][dispatch] create_run FAILED scheduled_run_id=%d err=%v", c.ScheduledRunID, err)
			_ = d.repo.MarkError(ctx, d.cfg.ProjectID, c.ScheduledRunID, "create_run_failed", 0)
			errorsCount++
			continue
		}

		if res.Status == "dispatched" && res.RunID != "" {
			// consume budget after run exists (writes budget_ledger with reason runsched.consume)
			if err := d.repo.BudgetConsumeV19(ctx, d.cfg.ProjectID, c.ScheduledRunID, res.RunID, traceID); err != nil {
				// do not throw: leave run created; repair later
				log.Printf("[runschedsvc][gate] budget consume FAILED run_id=%s scheduled_run_id=%d err=%v", res.RunID, c.ScheduledRunID, err)
			}
			created++
			log.Printf("[runschedsvc][dispatch] created run_id=%s scheduled_run_id=%d", res.RunID, c.ScheduledRunID)
		} else {
			log.Printf("[runschedsvc][dispatch] create_run returned status=%s run_id=%s scheduled_run_id=%d",
				res.Status, res.RunID, c.ScheduledRunID,
			)
		}
	}

	log.Printf("[runschedsvc][dispatch] summary created=%d skipped=%d errors=%d", created, skipped, errorsCount)
	return created, skipped, nil
}