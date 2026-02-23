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
)

const BuildTag = "ak-go-worker-v4.2-claim-v4.3-evidence"

func main() {
	ctx := context.Background()

	dsn := mustEnv("AK_DB_DSN") // e.g. "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable"
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

	// self-signed CA (dev/integration only): mainがpolicyに注入する。fetcherはenvを読まない。
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

	// ---- worker fetcher ----
	fetcher := &worker.PISAGHTTPFetcher{
		Policy:    policy,
		UserAgent: "ak-go-worker/" + BuildTag,
		// ClientはnilでOK（内部で pisag.NewClient(policy)）
	}

	cfg := worker.Config{
		WorkerID:         workerID,
		Poll:             poll,
		EvidenceMaxBytes: maxBytes,
		EvidenceBaseDir:  baseDir,
	}
	// claim style
	switch claimStyle {
	case "cte_skip_locked":
		cfg.ClaimStyle = "cte_skip_locked"
	case "update_returning":
		cfg.ClaimStyle = "update_returning"
	default:
		log.Fatalf("unknown AK_CLAIM_STYLE: %s", claimStyle)
	}

	log.Printf("boot: build=%s worker_id=%s poll=%s claim_style=%s dsn=%s evidence_dir=%s max_bytes=%d",
		BuildTag, workerID, poll, claimStyle, maskDSN(dsn), baseDir, maxBytes)

	w := worker.NewWorker(store, fetcher, log.Default(), cfg)

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
	rawID := strings.TrimSpace(os.Getenv("AK_WORKER_ID"))
	host, _ := os.Hostname()
	if rawID == "" {
		rawID = host
	}
	if !strings.Contains(rawID, "@") {
		return fmt.Sprintf("%s@%s", rawID, host)
	}
	return rawID
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

func maskDSN(dsn string) string {
	// ざっくり: passwordを隠す
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if j := strings.Index(rest, "@"); j >= 0 {
			cred := rest[:j]
			if k := strings.Index(cred, ":"); k >= 0 {
				return dsn[:i+3] + cred[:k+1] + "****" + rest[j:]
			}
		}
	}
	return dsn
}