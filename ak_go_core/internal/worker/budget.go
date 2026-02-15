package worker

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GateAndReserveBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	cost int64,
	reasonReserve string,
) (blockedEvent string, blockedPayload map[string]any, err error) {

	runID = NormalizeRunID(runID)
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

	if perRunLimit > 0 && cost > perRunLimit {
		return "run.blocked.budget", map[string]any{
			"project_id":    projectID,
			"reason":        "per_run_limit_exceeded",
			"cost":          cost,
			"per_run_limit": perRunLimit,
			"daily_limit":   dailyLimit,
			"monthly_limit": monthlyLimit,
		}, nil
	}

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
			"project_id":  projectID,
			"reason":      "daily_limit_exceeded",
			"cost":        cost,
			"spent_today": spentToday,
			"daily_limit": dailyLimit,
		}, nil
	}

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
		}, nil
	}

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

func ReleaseBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	amount int64,
	reasonRelease string,
) error {
	runID = NormalizeRunID(runID)
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

func CaptureBudgetTx(
	ctx context.Context,
	db *pgxpool.Pool,
	runID, traceID, projectID string,
	amount int64,
	reasonCapture string,
) error {
	runID = NormalizeRunID(runID)
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
