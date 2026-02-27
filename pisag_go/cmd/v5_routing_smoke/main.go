package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("DATABASE_URL")
	projectID := getenv("AK_PROJECT_ID", "akproj_0000000000000000000")

	db := mustOpenDB(dsn)
	defer db.Close()

	traceID := mustUUIDText(ctx, db)
	runID := mustCreateRun(ctx, db, projectID, traceID, "v5")

	log.Printf("[v5_smoke] project_id=%s trace_id=%s run_id=%s", projectID, traceID, runID)

	// Ensure at least 1 provider + 1 route exists for this project/region/currency/method.
	region := "JP"
	currency := "JPY"
	paymentMethod := "card"

	providerID, err := ensureProvider(ctx, db, projectID, "stripe")
	must(err)
	routeID, err := ensureProviderRoute(ctx, db, projectID, providerID, region, currency, paymentMethod)
	must(err)

	log.Printf("[v5_smoke] provider_id=%s route_id=%s", providerID, routeID)

	// Instantiate repos (v5)
	providersRepo := postgres.NewProvidersRepoV5(db)
	routesRepo := postgres.NewProviderRoutesRepoV5(db)
	metricsRepo := postgres.NewRoutingMetricsRepoV5(db)
	decisionsRepo := postgres.NewRouteDecisionsRepoV5(db)

	// Evidence repo (v18)
	// NOTE: This ctor name must match your existing file postgres/v18_evidence_repo.go
	// If your ctor name differs, rename here accordingly.
	evidenceRepo := postgres.NewEvidenceV18Repository(db)

	// Usecases
	previewUC := usecase.NewRoutingPreviewUsecaseV5(providersRepo, routesRepo, metricsRepo, evidenceRepo)
	commitUC := usecase.NewRoutingCommitUsecaseV5(previewUC, decisionsRepo)

	// -----------------------------
	// Preview
	// -----------------------------
	prevIn := run.RoutingPreviewInput{
		RoutingInput: run.RoutingInput{
			ProjectID: projectID,

			SubjectType:       "payment_intent",
			SubjectInternalID: "sub_internal_demo_0001",

			Region:        region,
			Currency:      currency,
			PaymentMethod: paymentMethod,
			AmountMinor:   1000,

			ConstraintsJSON: []byte(`{}`),

			PolicyVersion:   "p5-default",
			PipelineVersion: "v5",
			RoutingVersion:  "v5",

			TraceID: traceID,
			RunID:   runID,
		},
	}

	prevOut, err := previewUC.Handle(ctx, prevIn)
	must(err)

	log.Printf("[v5_smoke] preview status=%s fingerprint=%s why_evidence_ref=%s suggested_route=%v",
		prevOut.Status, prevOut.InputFingerprint, prevOut.WhyEvidenceRef, prevOut.SuggestedRouteID)

	// -----------------------------
	// Commit (accept suggested)
	// -----------------------------
	commitIn := run.RoutingCommitInput{
		RoutingInput: run.RoutingInput{
			ProjectID: projectID,

			SubjectType:       "payment_intent",
			SubjectInternalID: "sub_internal_demo_0001",

			Region:        region,
			Currency:      currency,
			PaymentMethod: paymentMethod,
			AmountMinor:   1000,

			ConstraintsJSON: []byte(`{}`),

			PolicyVersion:   "p5-default",
			PipelineVersion: "v5",
			RoutingVersion:  "v5",

			TraceID: traceID,
			RunID:   runID,
		},

		ExpectedInputFingerprint: prevOut.InputFingerprint,
		AcceptSuggested:          true,
		OverrideRouteID:          nil,
	}

	commitOut, err := commitUC.Handle(ctx, commitIn)
	must(err)

	log.Printf("[v5_smoke] commit decision_id=%s status=%s chosen_route=%v utl_key=%s why_ref=%s",
		commitOut.DecisionID, commitOut.Status, commitOut.ChosenRouteID, commitOut.UtlCommitEventKey, commitOut.WhyEvidenceRef)

	if !strings.HasPrefix(commitOut.UtlCommitEventKey, "utl_internal:") {
		log.Fatalf("expected utl_commit_event_key to start with utl_internal:, got=%s", commitOut.UtlCommitEventKey)
	}

	// -----------------------------
	// Verify DB row exists
	// -----------------------------
	mustVerifyDecision(ctx, db, projectID, commitOut.DecisionID)

	log.Printf("OK: v5 routing smoke passed")
}

// ----------------------------- helpers -----------------------------

func mustOpenDB(dsn string) *sql.DB {
	db, err := sql.Open("pgx", dsn)
	must(err)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	must(db.Ping())
	return db
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("env %s is required", k)
	}
	return v
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustUUIDText(ctx context.Context, db *sql.DB) string {
	var s string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text;`).Scan(&s); err != nil {
		log.Fatal(err)
	}
	return s
}

func mustCreateRun(ctx context.Context, db *sql.DB, projectID, traceID, pipelineVersion string) string {
	var runID string
	// runs schema is from 0001_runs_v41.sql (run_id uuid, trace_id uuid)
	err := db.QueryRowContext(ctx, `
INSERT INTO public.runs(project_id, trace_id, pipeline_version, status, started_at)
VALUES ($1, $2::uuid, $3, 'running', now())
RETURNING run_id::text;
`, projectID, traceID, pipelineVersion).Scan(&runID)
	must(err)
	return runID
}

func ensureProvider(ctx context.Context, db *sql.DB, projectID, providerKey string) (string, error) {
	var providerID string
	err := db.QueryRowContext(ctx, `
SELECT provider_id::text
FROM public.providers
WHERE project_id=$1 AND provider_key=$2
LIMIT 1;
`, projectID, providerKey).Scan(&providerID)
	if err == nil && strings.TrimSpace(providerID) != "" {
		return providerID, nil
	}

	// insert
	err = db.QueryRowContext(ctx, `
INSERT INTO public.providers(project_id, provider_key, status, capabilities, meta)
VALUES ($1, $2, 'active', '{}'::jsonb, '{}'::jsonb)
RETURNING provider_id::text;
`, projectID, providerKey).Scan(&providerID)
	return providerID, err
}

func ensureProviderRoute(ctx context.Context, db *sql.DB, projectID, providerID, region, currency, method string) (string, error) {
	var routeID string

	// Try find existing active route
	err := db.QueryRowContext(ctx, `
SELECT route_id::text
FROM public.provider_routes
WHERE project_id=$1
  AND provider_id=$2::uuid
  AND status='active'
  AND region=$3 AND currency=$4 AND payment_method=$5
ORDER BY priority ASC
LIMIT 1;
`, projectID, providerID, region, currency, method).Scan(&routeID)
	if err == nil && strings.TrimSpace(routeID) != "" {
		return routeID, nil
	}

	weights := map[string]any{
		"success": 0.5,
		"cost":    0.3,
		"latency": 0.2,
	}
	wb, _ := json.Marshal(weights)

	// insert minimal route
	err = db.QueryRowContext(ctx, `
INSERT INTO public.provider_routes(
  project_id, provider_id, status, priority,
  region, currency, payment_method,
  constraints, weights, why_policy_ref, meta
)
VALUES (
  $1, $2::uuid, 'active', 100,
  $3, $4, $5,
  '{}'::jsonb, $6::jsonb, 'p5-default', '{}'::jsonb
)
RETURNING route_id::text;
`, projectID, providerID, region, currency, method, string(wb)).Scan(&routeID)
	return routeID, err
}

func mustVerifyDecision(ctx context.Context, db *sql.DB, projectID, decisionID string) {
	var got string
	err := db.QueryRowContext(ctx, `
SELECT utl_commit_event_key
FROM public.route_decisions
WHERE project_id=$1 AND decision_id=$2::uuid
LIMIT 1;
`, projectID, decisionID).Scan(&got)
	must(err)

	if !strings.HasPrefix(got, "utl_internal:") {
		log.Fatalf("db utl_commit_event_key must start with utl_internal:, got=%s", got)
	}
}