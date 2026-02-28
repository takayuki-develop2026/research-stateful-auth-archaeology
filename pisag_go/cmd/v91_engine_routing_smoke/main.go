package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("DATABASE_URL")
	projectID := getenv("AK_PROJECT_ID", "akproj_0000000000000000000")

	db := mustOpenDB(dsn)
	defer db.Close()

	// Ensure v5 prerequisites: provider + route
	region := "JP"
	currency := "JPY"
	paymentMethod := "card"
	providerID, err := ensureProvider(ctx, db, projectID, "stripe")
	must(err)
	routeID, err := ensureProviderRoute(ctx, db, projectID, providerID, region, currency, paymentMethod)
	must(err)
	log.Printf("[v91_smoke] provider_id=%s route_id=%s", providerID, routeID)

	// repos / usecases
	evidenceRepo := postgres.NewEvidenceV18Repository(db)

	// v5 routing usecases
	providersRepoV5 := postgres.NewProvidersRepoV5(db)
	routesRepoV5 := postgres.NewProviderRoutesRepoV5(db)
	metricsRepoV5 := postgres.NewRoutingMetricsRepoV5(db)
	decisionsRepoV5 := postgres.NewRouteDecisionsRepoV5(db)

	utlRepoV6 := postgres.NewUtlRepoV6(db)

	previewUC := usecase.NewRoutingPreviewUsecaseV5(providersRepoV5, routesRepoV5, metricsRepoV5, evidenceRepo)
	commitUC := usecase.NewRoutingCommitUsecaseV5(previewUC, decisionsRepoV5)
	commitToUtlUC := usecase.NewRoutingCommitToUtlV6Usecase(commitUC, utlRepoV6)

	// v9 engine
	engineRepo := postgres.NewEngineRepoV9(db)
	builder := usecase.NewRoutingV5EngineDecisionBuilder(commitToUtlUC)
	engineUC := usecase.NewEngineDecideUsecaseV9(engineRepo, evidenceRepo, builder)
	engineUC.CacheTTL = 2 * time.Minute

	nonce := time.Now().UnixNano()
	input := map[string]any{
		"subject_type":        "payment_intent",
		"subject_internal_id": fmt.Sprintf("sub_internal_v91_%d", nonce),
		"region":              region,
		"currency":            currency,
		"payment_method":      paymentMethod,
		"amount_minor":        1234,
		"constraints": map[string]any{
			"nonce": nonce,
		},
		"accept_suggested": true,
	}
	inJSON, _ := json.Marshal(input)

	// 1st decide (cache miss guaranteed for this nonce)
	trace1 := mustUUIDText(ctx, db)
	run1 := mustCreateRun(ctx, db, projectID, trace1, "v9")
	in1 := usecase.EngineDecideInput{
		ProjectID: projectID,
		RunID:     run1,
		TraceID:   trace1,

		TaskType:        "routing_decide_v5",
		Mode:            "mode0_rule_only",
		PipelineVersion: "v9",
		PolicyVersion:   "p9-default",

		IdempotencyKey: fmt.Sprintf("engine_decide:v91_routing:%d:1", nonce),

		Principal: usecase.EnginePrincipal{
			ActorType: "system",
			ActorID:   "v91_smoke",
			Roles:     []string{"developer"},
		},
		InputJSON: inJSON,
	}

	out1, err := engineUC.Handle(ctx, in1)
	must(err)
	log.Printf("[v91_smoke] #1 status=%s cache_hit=%v engine_run_id=%s decision_id=%s type=%s",
		out1.Status, out1.CacheHit, out1.EngineRunID, out1.DecisionID, out1.DecisionType)

	if out1.CacheHit {
		log.Fatalf("expected cache_hit=false on first call (nonce=%d)", nonce)
	}
	if out1.DecisionType != "route" && out1.DecisionType != "review_required" {
		log.Fatalf("unexpected decision_type=%s", out1.DecisionType)
	}

	// 2nd decide (same input/principal -> cache hit)
	trace2 := mustUUIDText(ctx, db)
	run2 := mustCreateRun(ctx, db, projectID, trace2, "v9")
	in2 := in1
	in2.RunID = run2
	in2.TraceID = trace2
	in2.IdempotencyKey = fmt.Sprintf("engine_decide:v91_routing:%d:2", nonce)

	out2, err := engineUC.Handle(ctx, in2)
	must(err)
	log.Printf("[v91_smoke] #2 status=%s cache_hit=%v engine_run_id=%s decision_id=%s type=%s",
		out2.Status, out2.CacheHit, out2.EngineRunID, out2.DecisionID, out2.DecisionType)

	if !out2.CacheHit {
		log.Fatalf("expected cache_hit=true on second call (nonce=%d)", nonce)
	}
	if out1.CacheKey != out2.CacheKey {
		log.Fatalf("expected same cache_key, got %s vs %s", out1.CacheKey, out2.CacheKey)
	}
	if out1.DecisionID != out2.DecisionID {
		log.Fatalf("expected same decision_id (cached), got %s vs %s", out1.DecisionID, out2.DecisionID)
	}

	// Verify DB side-effects by reading decision_ledger_v9.result_json
	utlEventKey, routingDecisionID := mustExtractKeysFromDecision(ctx, db, out1.DecisionID)
	mustVerifyUtlRow(ctx, db, projectID, utlEventKey)
	mustVerifyRouteDecisionRow(ctx, db, projectID, routingDecisionID)
	mustVerifyEngineCacheRow(ctx, db, projectID, out1.CacheKey)

	log.Printf("OK: v9.1 routing builder smoke passed (nonce=%d)", nonce)
}

// ----------------------------- DB verify helpers -----------------------------

func mustExtractKeysFromDecision(ctx context.Context, db *sql.DB, decisionID string) (utlEventKey string, routingDecisionID string) {
	const q = `
SELECT result_json::text
FROM public.decision_ledger_v9
WHERE decision_id=$1::uuid
LIMIT 1;
`
	var s string
	if err := db.QueryRowContext(ctx, q, decisionID).Scan(&s); err != nil {
		log.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		log.Fatal(err)
	}

	utl, _ := m["utl"].(map[string]any)
	if utl == nil {
		log.Fatalf("decision.result_json.utl is missing")
	}
	evk, _ := utl["event_key"].(string)
	if strings.TrimSpace(evk) == "" {
		log.Fatalf("decision.result_json.utl.event_key is missing")
	}

	rd, _ := m["route_decision"].(map[string]any)
	if rd == nil {
		log.Fatalf("decision.result_json.route_decision is missing")
	}
	rid, _ := rd["decision_id"].(string)
	if strings.TrimSpace(rid) == "" {
		log.Fatalf("decision.result_json.route_decision.decision_id is missing")
	}

	return evk, rid
}

func mustVerifyUtlRow(ctx context.Context, db *sql.DB, projectID, eventKey string) {
	var gotID int64
	var gotStatus string
	err := db.QueryRowContext(ctx, `
SELECT id, status
FROM public.universal_events_v6
WHERE project_id=$1 AND event_key=$2
LIMIT 1;
`, projectID, eventKey).Scan(&gotID, &gotStatus)
	if err != nil {
		log.Fatal(err)
	}
	if !strings.HasPrefix(eventKey, "utl_internal:") {
		log.Fatalf("expected utl_internal: event_key, got=%s", eventKey)
	}
	log.Printf("[v91_smoke] verified UTL id=%d status=%s", gotID, gotStatus)
}

func mustVerifyRouteDecisionRow(ctx context.Context, db *sql.DB, projectID, decisionID string) {
	var got string
	err := db.QueryRowContext(ctx, `
SELECT decision_id::text
FROM public.route_decisions
WHERE project_id=$1 AND decision_id=$2::uuid
LIMIT 1;
`, projectID, decisionID).Scan(&got)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[v91_smoke] verified route_decisions decision_id=%s", got)
}

func mustVerifyEngineCacheRow(ctx context.Context, db *sql.DB, projectID, cacheKey string) {
	var gotID int64
	err := db.QueryRowContext(ctx, `
SELECT id
FROM public.engine_cache_v9
WHERE project_id=$1 AND cache_key=$2::char(64)
LIMIT 1;
`, projectID, cacheKey).Scan(&gotID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[v91_smoke] verified engine_cache_v9 id=%d", gotID)
}

// ----------------------------- general helpers -----------------------------

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

	err = db.QueryRowContext(ctx, `
INSERT INTO public.providers(project_id, provider_key, status, capabilities, meta)
VALUES ($1, $2, 'active', '{}'::jsonb, '{}'::jsonb)
RETURNING provider_id::text;
`, projectID, providerKey).Scan(&providerID)
	return providerID, err
}

func ensureProviderRoute(ctx context.Context, db *sql.DB, projectID, providerID, region, currency, method string) (string, error) {
	var routeID string
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

	weights := map[string]any{"success": 0.5, "cost": 0.3, "latency": 0.2}
	wb, _ := json.Marshal(weights)

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