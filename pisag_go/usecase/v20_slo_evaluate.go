package usecase

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type V20SloEvaluateInput struct {
	ProjectID string
	TraceID   string

	SloID int64 // slo_definitions.id
	// if empty, use slo_definitions.window_kind
	WindowKind string // 7d|30d (optional)

	// evidence asset for evaluation (numbers, query summary, etc.)
	EvaluationEvidenceAssetID int64

	// evidence asset for incident summary if breach
	IncidentSummaryEvidenceAssetID int64

	// incident severity/type for breach
	IncidentSeverity string // P1|P2|P3|P4 (default P2)
	IncidentType     string // default "slo_breach"
}

type V20SloEvaluateOutput struct {
	ProjectID string
	TraceID   string

	SloID          int64
	EvaluationKey  string
	WindowStartUTC time.Time
	WindowEndUTC   time.Time

	Total   int64
	Success int64
	Sli     float64

	Target float64
	Status string // ok|warn|breach

	CreatedIncident bool
	IncidentID      *int64
}

type V20SloEvaluateUseCase struct {
	DB *sql.DB
}

func (uc *V20SloEvaluateUseCase) Handle(ctx context.Context, in V20SloEvaluateInput) (V20SloEvaluateOutput, error) {
	pid := strings.TrimSpace(in.ProjectID)
	if pid == "" {
		return V20SloEvaluateOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return V20SloEvaluateOutput{}, errors.New("trace_id is required")
	}
	if in.SloID <= 0 {
		return V20SloEvaluateOutput{}, errors.New("slo_id is required")
	}
	if in.EvaluationEvidenceAssetID <= 0 {
		return V20SloEvaluateOutput{}, errors.New("evaluation_evidence_asset_id is required")
	}
	if in.IncidentSummaryEvidenceAssetID <= 0 {
		return V20SloEvaluateOutput{}, errors.New("incident_summary_evidence_asset_id is required")
	}

	// 1) load slo definition
	var (
		windowKind string
		target     float64
		enabled    bool
	)
	err := uc.DB.QueryRowContext(ctx, `
SELECT window_kind, target::float8, enabled
FROM public.slo_definitions
WHERE id=$1 AND project_id=$2
LIMIT 1;
`, in.SloID, pid).Scan(&windowKind, &target, &enabled)
	if err != nil {
		return V20SloEvaluateOutput{}, err
	}
	if !enabled {
		return V20SloEvaluateOutput{}, errors.New("slo is disabled")
	}

	if strings.TrimSpace(in.WindowKind) != "" {
		windowKind = strings.TrimSpace(in.WindowKind)
	}

	windowEnd := time.Now().UTC()
	windowStart, err := windowStartFromKind(windowEnd, windowKind)
	if err != nil {
		return V20SloEvaluateOutput{}, err
	}

	evalKey := evaluationKey(pid, in.SloID, windowStart, windowEnd)

	// 2) aggregate runs
	var total, success int64
	err = uc.DB.QueryRowContext(ctx, `
SELECT
  COUNT(*) FILTER (WHERE status IN ('done','failed')) AS total,
  COUNT(*) FILTER (WHERE status = 'done') AS success
FROM public.runs
WHERE project_id = $1
  AND created_at >= $2
  AND created_at <  $3;
`, pid, windowStart, windowEnd).Scan(&total, &success)
	if err != nil {
		return V20SloEvaluateOutput{}, err
	}

	out := V20SloEvaluateOutput{
		ProjectID:       pid,
		TraceID:         strings.TrimSpace(in.TraceID),
		SloID:           in.SloID,
		EvaluationKey:   evalKey,
		WindowStartUTC:  windowStart,
		WindowEndUTC:    windowEnd,
		Total:           total,
		Success:         success,
		Target:          target,
		CreatedIncident: false,
	}

	var sli float64
	status := "warn" // default for insufficient data
	errorBudgetRemaining := 1.0

	if total > 0 {
		sli = float64(success) / float64(total)
		if sli >= target {
			status = "ok"
			errorBudgetRemaining = 1.0
		} else {
			status = "breach"
			// simple P0: remaining is (target - sli) distance in budget space (clamped)
			// you can refine later with true error-budget math.
			errorBudgetRemaining = clamp01(1.0 - (target - sli))
		}
	}
	out.Sli = sli
	out.Status = status

	// 3) upsert evaluation (unique: project_id + evaluation_key)
	_, err = uc.DB.ExecContext(ctx, `
INSERT INTO public.slo_evaluations(
  project_id, slo_id, evaluation_key,
  window_start_at_utc, window_end_at_utc,
  sli_value, error_budget_remaining, status,
  evaluated_at_utc, evaluation_evidence_asset_id
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),$9)
ON CONFLICT (project_id, evaluation_key) DO UPDATE
SET
  sli_value = EXCLUDED.sli_value,
  error_budget_remaining = EXCLUDED.error_budget_remaining,
  status = EXCLUDED.status,
  evaluated_at_utc = now(),
  evaluation_evidence_asset_id = EXCLUDED.evaluation_evidence_asset_id;
`, pid, in.SloID, evalKey, windowStart, windowEnd, sli, errorBudgetRemaining, status, in.EvaluationEvidenceAssetID)
	if err != nil {
		return V20SloEvaluateOutput{}, err
	}

	// 4) breach -> incident_create_v20 (dedupe by incident_key)
	if status == "breach" {
		sev := strings.TrimSpace(in.IncidentSeverity)
		if sev == "" {
			sev = "P2"
		}
		itype := strings.TrimSpace(in.IncidentType)
		if itype == "" {
			itype = "slo_breach"
		}

		incKey := incidentKey(pid, itype, "", windowStart, windowEnd)

		var incID int64
		var foundExisting bool

		// root_trace_id/root_run_id are NULL in P0 for aggregate incidents
		err = uc.DB.QueryRowContext(ctx, `
SELECT incident_id, found_existing
FROM public.incident_create_v20(
  $1,$2,'open',$3,$4,
  ($5)::uuid, ($6)::uuid,
  'slo', now(),
  $7, 0,
  NULL,
  $8
);
`,
			pid,
			incKey,
			sev,
			itype,
			nil, // root_trace_id
			nil, // root_run_id
			in.IncidentSummaryEvidenceAssetID,
			// idempotency key (optional) - safe deterministic key
			"eval:"+evalKey,
		).Scan(&incID, &foundExisting)
		if err != nil {
			return V20SloEvaluateOutput{}, err
		}
		out.CreatedIncident = !foundExisting
		out.IncidentID = &incID
	}

	return out, nil
}

func windowStartFromKind(end time.Time, kind string) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "7d":
		return end.Add(-7 * 24 * time.Hour), nil
	case "30d":
		return end.Add(-30 * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("invalid window_kind: %s", kind)
	}
}

func evaluationKey(projectID string, sloID int64, ws, we time.Time) string {
	base := fmt.Sprintf("%s|%d|%s|%s",
		strings.TrimSpace(projectID),
		sloID,
		ws.UTC().Format(time.RFC3339Nano),
		we.UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func incidentKey(projectID, incidentType, rootTraceID string, ws, we time.Time) string {
	rt := strings.TrimSpace(rootTraceID)
	if rt == "" {
		rt = "-"
	}
	base := fmt.Sprintf("%s|%s|%s|%s|%s",
		strings.TrimSpace(projectID),
		strings.TrimSpace(incidentType),
		rt,
		ws.UTC().Format(time.RFC3339Nano),
		we.UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}