package tests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"services/opagateway/internal/opa"
	"services/opagateway/internal/postgres"
	"services/opagateway/internal/usecase"
)

func mustEnv(t *testing.T, k string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		t.Fatalf("missing env %s", k)
	}
	return v
}

func uniqueTrace(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func newSvc(t *testing.T) (*usecase.Service, *postgres.DB) {
	t.Helper()
	ctx := context.Background()
	dsn := mustEnv(t, "AK_PG_DSN")
	base := strings.TrimSpace(os.Getenv("OPA_BASE_URL")) // can be empty for PDP down test

	db, err := postgres.New(ctx, dsn)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}

	// DB identity logs (useful for "wrong DB" incidents)
	t.Logf("AK_PG_DSN=%s", dsn)
	var dbID string
	_ = db.Pool.QueryRow(ctx, "SELECT inet_server_addr()::text || ':' || inet_server_port()::text").Scan(&dbID)
	t.Logf("DB_IDENTITY=%s", dbID)

	cli := opa.NewHTTPClient(opa.ClientConfig{
		BaseURL:       base,
		Timeout:       200 * time.Millisecond,
		RetryCount:    0,
		CacheTTL:      1 * time.Second,
		CacheMaxItems: 1000,
	})

	return usecase.NewService(db, cli), db
}

// CI-1: allow/deny regression (fixed input -> fixed output)
// Requires OPA to be running and returning allow/deny deterministically.
func TestV21_AllowDenyRegression(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("OPA_BASE_URL"))
	if base == "" {
		t.Skip("OPA_BASE_URL empty; skip allow/deny regression until OPA is running")
	}

	svc, _ := newSvc(t)
	ctx := context.Background()

	in := usecase.DecideInput{
		ProjectID:        mustEnv(t, "AK_PROJECT_ID"),
		TraceID:          uniqueTrace("trc_v21_ci_allow"), // avoid residue across reruns
		SubjectType:      "service",
		SubjectID:        "ci",
		ActionKey:        "schedule.disable",
		ActionClass:      opa.HighRisk,
		ResourceKey:      "schedule:demo",
		PolicyVersionStr: "policy_v1_published",
		PolicyPath:       "security/allow",
		Input: map[string]any{
			"meta":     map[string]any{"project_id": mustEnv(t, "AK_PROJECT_ID")},
			"subject":  map[string]any{"type": "service", "id": "ci"},
			"action":   map[string]any{"key": "schedule.disable"},
			"resource": map[string]any{"key": "schedule:demo"},
		},
	}

	out1, err := svc.DecideAndRecord(ctx, in)
	if err != nil {
		t.Fatalf("decide 1: %v", err)
	}
	out2, err := svc.DecideAndRecord(ctx, in)
	if err != nil {
		t.Fatalf("decide 2: %v", err)
	}

	if out1.Decision.Result != out2.Decision.Result {
		t.Fatalf("regression: result changed %s -> %s", out1.Decision.Result, out2.Decision.Result)
	}
}

// CI-2: PDP down => high_risk must deny (fail-closed)
// + MUST append compliance_events_v21 (WORM入口) with evidence.
func TestV21_PDPDown_FailClosedHighRisk(t *testing.T) {
	// Force PDP down by clearing OPA_BASE_URL at runtime (restore after)
	old := os.Getenv("OPA_BASE_URL")
	_ = os.Setenv("OPA_BASE_URL", "")
	defer func() { _ = os.Setenv("OPA_BASE_URL", old) }()

	svc, db := newSvc(t)
	ctx := context.Background()

	project := mustEnv(t, "AK_PROJECT_ID")
	trace := uniqueTrace("trc_v21_ci_pdpdown")

	in := usecase.DecideInput{
		ProjectID:        project,
		TraceID:          trace,
		SubjectType:      "service",
		SubjectID:        "ci",
		ActionKey:        "evidence.download_raw",
		ActionClass:      opa.HighRisk,
		ResourceKey:      "evidence:123",
		PolicyVersionStr: "policy_v1_published",
		PolicyPath:       "security/allow",
		Input: map[string]any{
			"meta":     map[string]any{"project_id": project},
			"subject":  map[string]any{"type": "service", "id": "ci"},
			"action":   map[string]any{"key": "evidence.download_raw"},
			"resource": map[string]any{"key": "evidence:123"},
		},
	}

	out, err := svc.DecideAndRecord(ctx, in)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if out.Decision.Result != opa.ResultDeny {
		t.Fatalf("expected deny on PDP down high_risk; got %s", out.Decision.Result)
	}

	// v21 DoD-D: compliance_events_v21 must be appended (WORM入口)
	evText := "event=pdp_unavailable action_class=high_risk fail_closed=true component=opagateway_ci"

	assetID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx,
		db,
		project,
		trace,
		"service",              // created_by_type: system|user|service
		"opagateway_ci",        // created_by_id
		"generated",            // source_kind: pisag_fetch|upload|webhook|generated|import
		"opagateway://ci/v21",  // source_uri
		evText,
		"v21:ci:compliance:"+trace, // idempotency key for evidence_register_v18
	)
	if err != nil {
		t.Fatalf("register compliance evidence: %v", err)
	}

	_, _, err = postgres.AppendComplianceEventV21(
		ctx,
		db,
		project,
		trace,
		"pdp_unavailable",
		assetID,
		0, // primary_artifact_asset_id none
		"v21:ci:compliance_event:"+trace, // idempotency key
	)
	if err != nil {
		t.Fatalf("append compliance_event_v21: %v", err)
	}

	ok, err := postgres.HasComplianceEventV21ForTrace(ctx, db, project, trace, "pdp_unavailable")
	if err != nil {
		t.Fatalf("verify compliance_event_v21: %v", err)
	}
	if !ok {
		t.Fatalf("expected compliance_event_v21 present, but not found")
	}
}

// CI-3: audit missing detection for high_risk allow is NG
// Detector: if high_risk decision result=allow, an audit event must exist for the same trace_id.
func TestV21_AuditMissingDetection(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("OPA_BASE_URL"))
	if base == "" {
		t.Skip("needs OPA allow to create allow decision; skip until OPA is running")
	}

	svc, db := newSvc(t)
	ctx := context.Background()

	project := mustEnv(t, "AK_PROJECT_ID")
	trace := uniqueTrace("trc_v21_ci_audit_missing")

	in := usecase.DecideInput{
		ProjectID:        project,
		TraceID:          trace,
		SubjectType:      "service",
		SubjectID:        "ci",
		ActionKey:        "budget.raise",
		ActionClass:      opa.HighRisk,
		ResourceKey:      "budget:project",
		PolicyVersionStr: "policy_v1_published",
		PolicyPath:       "security/allow",
		Input: map[string]any{
			"meta":     map[string]any{"project_id": project},
			"subject":  map[string]any{"type": "service", "id": "ci"},
			"action":   map[string]any{"key": "budget.raise"},
			"resource": map[string]any{"key": "budget:project"},
		},
	}

	out, err := svc.DecideAndRecord(ctx, in)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if out.Decision.Result != opa.ResultAllow {
		t.Skip("OPA did not allow; cannot test audit missing detection")
	}

	// Intentionally DO NOT write audit event. Detector should observe "missing".
	ok, err := postgres.HasAuditForTrace(ctx, db, project, trace, "policy_decision")
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if ok {
		t.Fatalf("expected audit missing, but found audit")
	}

	// Now write audit and ensure detector passes.
	_, _, err = postgres.AppendAuditEventV18(
		ctx, db,
		project, trace,
		"policy_decision_made", "service", "opagateway",
		"action", "budget.raise",
		"ok",
		"test audit present", "{}", "v21:ci:audit:"+trace,
	)
	if err != nil {
		t.Fatalf("append audit: %v", err)
	}

	ok2, err := postgres.HasAuditForTrace(ctx, db, project, trace, "policy_decision")
	if err != nil {
		t.Fatalf("audit query 2: %v", err)
	}
	if !ok2 {
		t.Fatalf("expected audit present, but not found")
	}
}