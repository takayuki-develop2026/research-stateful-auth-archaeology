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

	"example.com/pisag_go/postgres"
)

type V20SloEvaluateInput struct {
	ProjectID string
	TraceID   string

	SloID      int64
	WindowKind string // 7d|30d optional

	EvaluationEvidenceAssetID      int64
	IncidentSummaryEvidenceAssetID int64

	IncidentSeverity string
	IncidentType     string
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
	Status string

	CreatedIncident bool
	IncidentID      *int64
}

type V20SloEvaluateUseCase struct {
	DB      *sql.DB
	Now     func() time.Time
	V13Repo *postgres.V13Repository // ★追加（nilならv13記録スキップ）
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

	now := time.Now
	if uc.Now != nil {
		now = uc.Now
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

	// 2) bucketized window end
	windowEnd := bucketEndUTC(now().UTC(), windowKind)
	windowStart, err := windowStartFromKind(windowEnd, windowKind)
	if err != nil {
		return V20SloEvaluateOutput{}, err
	}

	evalKey := evaluationKey(pid, in.SloID, windowStart, windowEnd)

	// ---- v13 idempotency (P1): start
	var idemID int64
	if uc.V13Repo != nil {
		scope := "v20_slo_evaluate"
		key := "ak:idem:v20_slo_evaluate:" + shortHash(evalKey)
		start, ierr := uc.V13Repo.IdempotencyStart(ctx, pid, scope, key, evalKey)
		if ierr != nil {
			return V20SloEvaluateOutput{}, ierr
		}
		idemID = start.IdempotencyID
	}

	// 3) aggregate runs
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
		uc.idemFail(ctx, pid, idemID, err)
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
	status := "warn"
	errorBudgetRemaining := 1.0

	if total > 0 {
		sli = float64(success) / float64(total)
		if sli >= target {
			status = "ok"
			errorBudgetRemaining = 1.0
		} else {
			status = "breach"
			errorBudgetRemaining = clamp01(1.0 - (target - sli))
		}
	}
	out.Sli = sli
	out.Status = status

	// 4) upsert evaluation (EXECUTE ONLY)
	const up = `SELECT public.slo_evaluation_upsert_v20($1,$2,$3,$4,$5,$6,$7,$8,$9);`
	if _, err := uc.DB.ExecContext(ctx, up,
		pid, in.SloID, evalKey, windowStart, windowEnd,
		sli, errorBudgetRemaining, status, in.EvaluationEvidenceAssetID,
	); err != nil {
		uc.idemFail(ctx, pid, idemID, err)
		return V20SloEvaluateOutput{}, err
	}

	// 5) breach -> incident_create_v20 (already idempotent)
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
			pid, incKey, sev, itype,
			nil, nil,
			in.IncidentSummaryEvidenceAssetID,
			"eval:"+evalKey,
		).Scan(&incID, &foundExisting)
		if err != nil {
			uc.idemFail(ctx, pid, idemID, err)
			return V20SloEvaluateOutput{}, err
		}
		out.CreatedIncident = !foundExisting
		out.IncidentID = &incID
	}

	// ---- v13 idempotency finish
	if uc.V13Repo != nil && idemID > 0 {
		sum := fmt.Sprintf("ok status=%s total=%d success=%d", out.Status, out.Total, out.Success)
		_ = uc.V13Repo.IdempotencyFinish(ctx, pid, idemID, "succeeded", &sum, &in.EvaluationEvidenceAssetID, time.Now().UTC())
	}

	return out, nil
}

func (uc *V20SloEvaluateUseCase) idemFail(ctx context.Context, projectID string, idemID int64, err error) {
	if uc.V13Repo == nil || idemID <= 0 {
		return
	}
	msg := err.Error()
	if len(msg) > 240 {
		msg = msg[:240]
	}
	_ = uc.V13Repo.IdempotencyFinish(ctx, projectID, idemID, "failed", &msg, nil, time.Now().UTC())
}

// ---- helpers (unchanged) ----

func bucketEndUTC(t time.Time, kind string) time.Time {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "7d":
		return t.Truncate(time.Hour)
	case "30d":
		return t.Truncate(24 * time.Hour)
	default:
		return t.Truncate(time.Hour)
	}
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
