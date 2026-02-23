package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/pisag"
	"example.com/pisag_go/ports"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

const BuildTag = "ak_go_worker-v43-fixed-20260222"

func main() {
	ctx := context.Background()

	// DATABASE_URL を優先取得
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = mustEnv("AK_DB_DSN")
	}

	workerID := strings.TrimSpace(os.Getenv("AK_WORKER_ID"))
	if workerID == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = "unknown-host"
		}
		workerID = fmt.Sprintf("worker@%s", host)
	}

	poll := envDuration("AK_WORKER_POLL", 500*time.Millisecond)
	claimStyle := envDefault("AK_CLAIM_STYLE", string(postgres.ClaimStyleCTE))
	evidenceDir := envDefault("AK_EVIDENCE_DIR", "./var/evidence")
	maxBodyBytes := envInt64("AK_MAX_BODY_BYTES", 5<<20)

	// --- DB connect ---
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	// --- Repos ---
	claimRepo := postgres.NewRunInputClaimRepository(db)
	traceRepo := postgres.NewRunTraceRepository(db)
	evidenceRepo := postgres.NewEvidenceRepository(db)

	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{Host: "oracle.singularity.local", Port: 443, PathPrefixes: []string{"/"}},
		},
		AllowCIDRs:   []string{"172.16.0.0/12"},
		MaxRedirects: 3,
		Timeout:      15 * time.Second,
	}

	policy, err = withOracleSelfSignedFromEnv(policy)
	if err != nil {
		log.Fatalf("load oracle CA error: %v", err)
	}

	pClient, err := pisag.NewClient(policy)
	if err != nil {
		log.Fatalf("pisag.NewClient error: %v", err)
	}

	// Fetcher: FetchBytes を呼ぶため具象型のポインタとして定義
	fetcher := &usecase.PISAGFetcher{
		Policy:       policy,
		Client:       pClient,
		MaxBodyBytes: maxBodyBytes,
		UserAgent:    "ak-go-worker/v4.3",
	}

	log.Printf("boot build=%s worker_id=%s poll=%s", BuildTag, workerID, poll)

	for {
		in, err := claimRepo.ClaimNextRunInput(ctx, workerID, postgres.ClaimStyle(claimStyle))
		if err != nil {
			log.Printf("claim error: %v", err)
			time.Sleep(poll)
			continue
		}
		if in == nil {
			time.Sleep(poll)
			continue
		}

		if err := handleOne(ctx, workerID, *in, traceRepo, fetcher, evidenceRepo, evidenceDir, claimRepo); err != nil {
			log.Printf("handle error (input_id=%d run_id=%s): %v", in.ID, in.RunID, err)
			continue
		}
	}
}

func handleOne(
	ctx context.Context,
	workerID string,
	in run.RunInput,
	traceRepo *postgres.RunTraceRepository,
	fetcher *usecase.PISAGFetcher,
	evidenceRepo *postgres.EvidenceRepository,
	evidenceDir string,
	claimRepo *postgres.RunInputClaimRepository,
) error {
	inputID := in.ID
	runID := in.RunID

	// 1. trace_id 取得
	traceID, err := traceRepo.GetTraceID(ctx, runID)
	if err != nil {
		_ = claimRepo.MarkRunInputRetry(ctx, inputID, workerID, "trace_lookup_failed", err.Error())
		return err
	}

	// 2. Fetch (Body + Result)
	body, res, err := fetcher.FetchBytes(ctx, in.TargetURL)
	if err != nil {
		code := "fetch_failed"
		if errorsIsDenied(err) {
			code = "fetch_denied"
		}
		_ = claimRepo.MarkRunInputRetry(ctx, inputID, workerID, code, err.Error())
		return err
	}

	if res.StatusCode != 200 {
		code := "http_status_not_ok"
		msg := fmt.Sprintf("status=%d", res.StatusCode)
		_ = claimRepo.MarkRunInputRetry(ctx, inputID, workerID, code, msg)
		return fmt.Errorf("%s: %s", code, msg)
	}

	// 3. Evidence File Persistence (v4.3 requirement)
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	storedPath, err := writeEvidenceFile(evidenceDir, runID, sha, body)
	if err != nil {
		_ = claimRepo.MarkRunInputRetry(ctx, inputID, workerID, "evidence_write_failed", err.Error())
		return err
	}

	// 4. DB Evidence Registration
	ev := run.EvidenceAsset{
		RunID:       runID,
		TraceID:     traceID,
		Kind:        "fetch_body",
		ContentType: res.ContentType,
		ByteSize:    res.BodySize,
		SHA256:      sha,
		FinalURL:    res.FinalURL,
		StoredPath:  storedPath,
	}
	if err := evidenceRepo.InsertEvidence(ctx, ev); err != nil {
		_ = claimRepo.MarkRunInputRetry(ctx, inputID, workerID, "evidence_insert_failed", err.Error())
		return err
	}

	// 5. Done (inputID int64 を使用)
	if err := claimRepo.MarkRunInputDone(ctx, inputID, workerID); err != nil {
		return err
	}

	log.Printf("done run_id=%s trace_id=%s size=%d path=%s", runID, traceID, res.BodySize, storedPath)
	return nil
}

func writeEvidenceFile(baseDir, runID, sha string, b []byte) (string, error) {
	dir := filepath.Join(baseDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s.bin", sha)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	// DB保存用にポータブルなパスを返す
	return filepath.ToSlash(filepath.Join("var", "evidence", runID, filename)), nil
}

func withOracleSelfSignedFromEnv(p ports.Policy) (ports.Policy, error) {
	caPath := strings.TrimSpace(os.Getenv("ORACLE_CA_PATH"))
	if caPath == "" {
		return p, nil
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return p, fmt.Errorf("read oracle CA: %w", err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(pem); !ok {
		return p, fmt.Errorf("append oracle CA failed")
	}
	p.TLSRootCAs = pool
	return p, nil
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func envDefault(k, def string) string {
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
	var x int64
	_, err := fmt.Sscanf(v, "%d", &x)
	if err != nil {
		return def
	}
	return x
}

func errorsIsDenied(err error) bool {
	return err != nil && strings.Contains(err.Error(), "denied")
}