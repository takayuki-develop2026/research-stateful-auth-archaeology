package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/pisag_go/ports"
	"example.com/pisag_go/usecase"
)

func TestIntegration_PISAGFetcher_Oracle(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("set RUN_INTEGRATION=1 to run")
	}

	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{
				Host:         "oracle.singularity.local",
				Port:         443,
				PathPrefixes: []string{"/"},
			},
		},
		// dev / integration only（docker network）
		AllowCIDRs: []string{
			"127.0.0.1/32",  // ← これを追加！ (Macローカルホスト)
			"172.16.0.0/12", // contains 172.19.0.0/16
		},
		MaxRedirects: 3,
		Timeout:      30 * time.Second,
	}

	// ★ self-signed を許可するには CA を明示する（InsecureSkipVerifyは禁止）
	policy2, err := usecase.WithOracleSelfSigned(policy)
	if err != nil {
		t.Fatalf("failed to load oracle CA: %v (set ORACLE_CA_PATH if needed)", err)
	}

	f := usecase.PISAGFetcher{
		Policy:       policy2,
		MaxBodyBytes: 1024 * 1024, // 1MB
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	res, err := f.Fetch(ctx, "https://oracle.singularity.local/pricing_v1.json")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if res.BodySize <= 0 {
		t.Fatalf("expected body_size > 0")
	}
}