package main

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/internal/worker"
	"example.com/pisag_go/ports"
	"example.com/pisag_go/postgres"
)

const BuildTag = "ak-go-worker-v4.5-manifest-links"

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN") // e.g. "postgres://ak_worker:...@127.0.0.1:5433/ak?sslmode=disable"
	workerID := buildWorkerID()

	poll := envDuration("AK_WORKER_POLL", 500*time.Millisecond)
	maxBytes := envInt64("AK_EVIDENCE_MAX_BYTES", 5<<20)
	baseDir := envString("AK_EVIDENCE_DIR", "./var/evidence")
	claimStyle := envString("AK_CLAIM_STYLE", "cte_skip_locked") // or "update_returning"

	// ---- PISAG Policy (single source of truth for allow/deny) ----
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{
				Host:         "oracle.singularity.local",
				Port:         443,
				PathPrefixes: []string{"/"},
			},
		},
		AllowCIDRs: []string{
			"172.16.0.0/12",
			"172.18.0.0/16",
			"172.19.0.0/16",
			"127.0.0.1/32", // dev only
		},
		MaxRedirects: 3,
		Timeout:      30 * time.Second,
	}

	// self-signed CA (dev/integration only): main injects into policy (fetcher does NOT read env).
	if caPath := strings.TrimSpace(os.Getenv("ORACLE_CA_PATH")); caPath != "" {
		pool, err := loadCertPoolAppendSystem(caPath)
		if err != nil {
			log.Fatalf("load oracle CA failed: %v", err)
		}
		policy.TLSRootCAs = pool
		log.Printf("oracle CA loaded: %s", caPath)
	}

	// ---- DB connect ----
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	store := worker.NewStore(db)

	// ★ v13 repo (DLQ/idempotency/contracts) for worker
	// Worker will call SECURITY DEFINER functions (EXECUTE ONLY), so SELECT権限は不要。
	v13repo := postgres.NewV13Repository(db)

	// ---- worker fetcher ----
	fetcher := &worker.PISAGHTTPFetcher{
		Policy:    policy,
		UserAgent: "ak-go-worker/" + BuildTag,
		// Client is nil OK (internal pisag.NewClient(policy))
	}

	cfg := worker.Config{
		WorkerID:         workerID,
		Poll:             poll,
		EvidenceMaxBytes: maxBytes,
		EvidenceBaseDir:  baseDir,
		IdleLogEvery:     10,
	}

	// claim style (fixed enum)
	switch claimStyle {
	case "cte_skip_locked":
		cfg.ClaimStyle = postgres.ClaimStyleCTE
	case "update_returning":
		cfg.ClaimStyle = postgres.ClaimStyleUpdateReturning
	default:
		log.Fatalf("unknown AK_CLAIM_STYLE: %s", claimStyle)
	}

	log.Printf("boot: build=%s worker_id=%s poll=%s claim_style=%s evidence_dir=%s max_bytes=%d",
		BuildTag, workerID, poll, claimStyle, baseDir, maxBytes)

	// ★変更：NewWorker に v13repo を渡す（worker.go 側のシグネチャ変更が前提）
	w := worker.NewWorker(store, fetcher, log.Default(), cfg, v13repo)

	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("worker stopped: %v", err)
	}
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func envString(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envDuration(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func envInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	var n int64
	_, _ = fmt.Sscanf(v, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func buildWorkerID() string {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)

	raw := strings.TrimSpace(os.Getenv("AK_WORKER_ID"))
	if raw == "" {
		if host == "" {
			return "worker"
		}
		return host
	}

	if strings.Contains(raw, "@") {
		return raw
	}

	if host == "" {
		return raw
	}
	if raw == host {
		return host
	}
	return fmt.Sprintf("%s@%s", raw, host)
}

func loadCertPoolAppendSystem(pemPath string) (*x509.CertPool, error) {
	b, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, fmt.Errorf("read CA pem: %w", err)
	}
	sys, _ := x509.SystemCertPool()
	if sys == nil {
		sys = x509.NewCertPool()
	}
	if ok := sys.AppendCertsFromPEM(b); !ok {
		return nil, fmt.Errorf("no certs appended from PEM: %s", pemPath)
	}
	return sys, nil
}