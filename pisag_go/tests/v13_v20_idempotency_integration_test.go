package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

// NOTE: requires DATABASE_URL, and assumes there is at least one project + evidence_asset.
// Also assumes v20 schema + v13 schema are migrated.
func TestV13_V20_Idempotency_SloEvaluate_Dedupe(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var projectID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM public.projects ORDER BY id LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var evidenceID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM public.evidence_assets ORDER BY id LIMIT 1`).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}

	// ensure SLO exists
	var sloID int64
	err = db.QueryRowContext(ctx, `
SELECT id FROM public.slo_definitions WHERE project_id=$1 ORDER BY id LIMIT 1
`, projectID).Scan(&sloID)
	if err != nil {
		if err := db.QueryRowContext(ctx, `
INSERT INTO public.slo_definitions(
  project_id, name, enabled, window_kind, target,
  slo_spec_evidence_asset_id, severity_policy_evidence_asset_id, alert_policy_evidence_asset_id,
  created_by_type, created_by_id
) VALUES ($1,'runs_success_rate_7d',true,'7d',0.99,$2,$2,$2,'system','test')
RETURNING id;
`, projectID, evidenceID).Scan(&sloID); err != nil {
			t.Fatal(err)
		}
	}

	// inject one failed run to ensure breach
	if _, err := db.ExecContext(ctx, `
INSERT INTO public.runs(project_id, pipeline_version, status, started_at, finished_at, created_at)
VALUES ($1,'v4.1','failed',now(),now(),now());
`, projectID); err != nil {
		t.Fatal(err)
	}

	v13repo := postgres.NewV13Repository(db)

	// fixed time (hour bucket)
	fixedNow := time.Date(2026, 2, 26, 1, 23, 45, 0, time.UTC)

	uc := usecase.V20SloEvaluateUseCase{
		DB:      db,
		V13Repo: v13repo,
		Now: func() time.Time {
			return fixedNow
		},
	}

	// run twice
	out1, err := uc.Handle(ctx, usecase.V20SloEvaluateInput{
		ProjectID:                     projectID,
		TraceID:                       "test-trace",
		SloID:                         sloID,
		EvaluationEvidenceAssetID:      evidenceID,
		IncidentSummaryEvidenceAssetID: evidenceID,
		IncidentSeverity:              "P2",
		IncidentType:                  "slo_breach",
	})
	if err != nil {
		t.Fatal(err)
	}
	out2, err := uc.Handle(ctx, usecase.V20SloEvaluateInput{
		ProjectID:                     projectID,
		TraceID:                       "test-trace",
		SloID:                         sloID,
		EvaluationEvidenceAssetID:      evidenceID,
		IncidentSummaryEvidenceAssetID: evidenceID,
		IncidentSeverity:              "P2",
		IncidentType:                  "slo_breach",
	})
	if err != nil {
		t.Fatal(err)
	}

	if out1.EvaluationKey != out2.EvaluationKey {
		t.Fatalf("evaluation_key must be stable: %s != %s", out1.EvaluationKey, out2.EvaluationKey)
	}
	if out1.Status != "breach" || out2.Status != "breach" {
		t.Fatalf("expected breach, got %s / %s", out1.Status, out2.Status)
	}
	if out2.CreatedIncident {
		t.Fatalf("second run should not create incident (dedupe expected)")
	}

	// v13 idempotency: exactly 1 record for v20_slo_evaluate at latest key
	var cnt int64
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM public.idempotency_records_v13
WHERE project_id=$1 AND scope='v20_slo_evaluate'
  AND idempotency_key LIKE 'ak:idem:v20_slo_evaluate:%'
  AND status='succeeded';
`, projectID).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt <= 0 {
		t.Fatalf("expected v20_slo_evaluate idempotency records to exist")
	}
}