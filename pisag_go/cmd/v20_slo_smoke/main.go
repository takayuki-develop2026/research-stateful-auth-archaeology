package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbURL := mustEnv("DATABASE_URL")
	db, err := postgres.Open(dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectID := firstNonEmpty(os.Getenv("PROJECT_ID"), mustQueryString(ctx, db, `SELECT id FROM public.projects ORDER BY id LIMIT 1`))
	evidenceID := mustQueryInt64(ctx, db, `SELECT id FROM public.evidence_assets ORDER BY id LIMIT 1`)
	if strings.TrimSpace(os.Getenv("EVIDENCE_ASSET_ID")) != "" {
		evidenceID = mustParseInt64("EVIDENCE_ASSET_ID")
	}

	// Ensure at least one SLO exists; if not, create one.
	sloID := ensureSlo(ctx, db, projectID, evidenceID)

	traceID := "v20-slo-smoke"

	// ★ v13 repo injection
	v13repo := postgres.NewV13Repository(db)

	uc := usecase.V20SloEvaluateUseCase{
		DB:      db,
		V13Repo: v13repo,
	}

	out, err := uc.Handle(ctx, usecase.V20SloEvaluateInput{
		ProjectID:                      projectID,
		TraceID:                        traceID,
		SloID:                          sloID,
		WindowKind:                     "", // use definition
		EvaluationEvidenceAssetID:      evidenceID,
		IncidentSummaryEvidenceAssetID: evidenceID,
		IncidentSeverity:               "P2",
		IncidentType:                   "slo_breach",
	})
	if err != nil {
		log.Fatalf("v20_slo_evaluate failed: %v", err)
	}

	log.Printf("[v20_slo_smoke] slo_id=%d window=[%s..%s] total=%d success=%d sli=%.6f target=%.6f status=%s",
		out.SloID, out.WindowStartUTC.Format(time.RFC3339), out.WindowEndUTC.Format(time.RFC3339),
		out.Total, out.Success, out.Sli, out.Target, out.Status,
	)
	if out.IncidentID != nil {
		log.Printf("[v20_slo_smoke] incident_id=%d created=%v", *out.IncidentID, out.CreatedIncident)
	} else {
		log.Printf("[v20_slo_smoke] incident: none")
	}
}

func ensureSlo(ctx context.Context, db *sql.DB, projectID string, evidenceID int64) int64 {
	var id int64
	err := db.QueryRowContext(ctx, `
SELECT id
FROM public.slo_definitions
WHERE project_id=$1
ORDER BY id ASC
LIMIT 1;
`, projectID).Scan(&id)
	if err == nil && id > 0 {
		return id
	}

	// create minimal SLO: 99% success rate, 7d window, enabled
	err = db.QueryRowContext(ctx, `
INSERT INTO public.slo_definitions(
  project_id, name, enabled, window_kind, target,
  slo_spec_evidence_asset_id, severity_policy_evidence_asset_id, alert_policy_evidence_asset_id,
  created_by_type, created_by_id
)
VALUES ($1, 'runs_success_rate_7d', true, '7d', 0.99,
        $2, $2, $2,
        'system', 'v20_slo_smoke')
RETURNING id;
`, projectID, evidenceID).Scan(&id)
	if err != nil {
		log.Fatalf("create slo_definitions failed: %v", err)
	}
	return id
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func firstNonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		return v
	}
	return fallback
}

func mustQueryString(ctx context.Context, db *sql.DB, q string) string {
	var s string
	if err := db.QueryRowContext(ctx, q).Scan(&s); err != nil {
		log.Fatalf("query string failed: %v (sql=%s)", err, q)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		log.Fatalf("query returned empty string (sql=%s)", q)
	}
	return s
}

func mustQueryInt64(ctx context.Context, db *sql.DB, q string) int64 {
	var n int64
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		log.Fatalf("query int64 failed: %v (sql=%s)", err, q)
	}
	if n <= 0 {
		log.Fatalf("query returned invalid int64 (sql=%s)", q)
	}
	return n
}

func mustParseInt64(k string) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	var n int64
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n <= 0 {
		log.Fatalf("invalid %s: %q", k, v)
	}
	return n
}
