// tests/v20_slo_evaluate_integration_test.go
package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/usecase"
)

func TestV20SloEvaluate_DedupeWithinBucket(t *testing.T) {
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

	// ensure slo exists (enabled=true)
	var sloID int64
	err = db.QueryRowContext(ctx, `
SELECT id FROM public.slo_definitions WHERE project_id=$1 ORDER BY id LIMIT 1
`, projectID).Scan(&sloID)
	if err != nil {
		// create minimal
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

	// Inject 1 failed run inside window
	if _, err := db.ExecContext(ctx, `
INSERT INTO public.runs(project_id, pipeline_version, status, started_at, finished_at, created_at)
VALUES ($1,'v4.1','failed',now(),now(),now());
`, projectID); err != nil {
		t.Fatal(err)
	}

	fixedNow := time.Date(2026, 2, 26, 5, 0, 0, 0, time.UTC) // hour bucket fixed

	uc := usecase.V20SloEvaluateUseCase{
		DB: db,
		Now: func() time.Time {
			return fixedNow
		},
	}

	// Run twice in same bucket
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
		t.Fatalf("evaluation_key must be stable within bucket: %s != %s", out1.EvaluationKey, out2.EvaluationKey)
	}
	if out1.Status != "breach" || out2.Status != "breach" {
		t.Fatalf("expected breach, got %s / %s", out1.Status, out2.Status)
	}

	// evaluations should be 1 row for that key
	var evalCount int64
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM public.slo_evaluations WHERE project_id=$1 AND evaluation_key=$2
`, projectID, out1.EvaluationKey).Scan(&evalCount); err != nil {
		t.Fatal(err)
	}
	if evalCount != 1 {
		t.Fatalf("expected 1 slo_evaluation row, got %d", evalCount)
	}

	// incidents should not multiply within same bucket for same eval key (idempotency + stable incident_key)
	// Check that the latest incident_key count is 1
	var incKey string
	// recompute the incident key via DB lookup from incidents (latest)
	if err := db.QueryRowContext(ctx, `
SELECT incident_key FROM public.incidents
WHERE project_id=$1 AND incident_type='slo_breach'
ORDER BY id DESC LIMIT 1
`, projectID).Scan(&incKey); err != nil {
		t.Fatal(err)
	}
	var incCount int64
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM public.incidents WHERE project_id=$1 AND incident_key=$2
`, projectID, incKey).Scan(&incCount); err != nil {
		t.Fatal(err)
	}
	if incCount != 1 {
		t.Fatalf("expected 1 incident row for same incident_key, got %d", incCount)
	}
}