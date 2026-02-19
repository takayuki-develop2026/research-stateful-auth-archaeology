package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"example.com/ak_go_core/internal/infra/db"
	redisx "example.com/ak_go_core/internal/infra/redis"
	"example.com/ak_go_core/internal/worker"

	"example.com/pisag_go/pisag"
    "example.com/pisag_go/ports" // Policy型のために追加
)

const BuildTag = "worker-refactor-20260215-01"

func main() {
	dsn := worker.MustEnv("AK_DB_DSN")

	rawID := strings.TrimSpace(os.Getenv("AK_WORKER_ID"))
	if rawID == "" {
		rawID = strings.TrimSpace(os.Getenv("AK_WORKER_NAME"))
	}
	host := worker.HostnameFallback()
	workerID := rawID
	if workerID == "" {
		workerID = host
	}
	if !strings.Contains(workerID, "@") {
		workerID = fmt.Sprintf("%s@%s", workerID, host)
	}

	poll := worker.GetenvDurationMS("AK_WORKER_POLL_MS", 500*time.Millisecond)
	cost := worker.GetenvInt64("AK_RUN_COST", 10)
	redisAddr := worker.GetenvDefault("AK_REDIS_ADDR", "ak_redis:6379")

	ctx := context.Background()

	// ---- PISAG v4: セキュリティ憲法（Policy）の構築 ----
	policy := ports.Policy{
		AllowedHosts: []ports.AllowedHost{
			{Host: "oracle.singularity.local"},
		},
		// Docker Composeの内部ネットワーク（シミュレーター）を許可
		AllowCIDRs:   []string{"172.16.0.0/12", "172.18.0.0/16"}, 
		MaxRedirects: 3,
		Timeout:      10 * time.Second,
	}

	// ---- PISAG v4: 物理ロックされたClientの生成 ----
	pClient, err := pisag.NewClient(policy)
	if err != nil {
		log.Fatalf("PISAG init error: %v", err)
	}

	pool, err := db.NewPool(ctx, db.Config{
		DSN:             dsn,
		MaxConns:        10,
		MinConns:        1,
		MaxConnIdle:     5 * time.Minute,
		MaxConnLifetime: 30 * time.Minute,
	})
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer pool.Close()

	rdb := redisx.NewClient(redisx.Config{Addr: redisAddr})
	defer func() { _ = rdb.Close() }()

	log.Printf("AK Go Worker booted with PISAG. build=%s worker_id=%s redis=%s poll=%s cost=%d", BuildTag, workerID, redisAddr, poll, cost)

	store := worker.NewStore(pool)

	// NewWorkerにPISAG ClientとPolicyを注入
	w := worker.NewWorker(store, rdb, worker.Config{
		WorkerID: workerID,
		Poll:     poll,
		Cost:     cost,
	}, log.Default(), pClient, policy)

	if err := w.Run(ctx); err != nil {
		log.Printf("worker stopped: %v", err)
	}
}