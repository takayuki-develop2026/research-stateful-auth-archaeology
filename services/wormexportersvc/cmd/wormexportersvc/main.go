package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"services/wormexportersvc/internal/postgres"
	"services/wormexportersvc/internal/worm"
)

func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envBool(k string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDuration(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func main() {
	pgDsn := envOr("AK_PG_DSN", "")
	projectID := envOr("AK_PROJECT_ID", "")
	if pgDsn == "" || projectID == "" {
		log.Fatal("missing AK_PG_DSN or AK_PROJECT_ID")
	}

	cfg := worm.Config{
		ProjectID:       projectID,
		Sink:            envOr("WORM_SINK", "localfile"),
		OutDir:          envOr("WORM_OUT_DIR", "/var/wormexporter/out"),
		Limit:           envInt("WORM_EXPORT_LIMIT", 50),
		RunOnce:         envBool("WORM_ONCE"),
		Every:           envDuration("WORM_EVERY", 1*time.Minute),
		StaleAfter:      envDuration("WORM_RECLAIM_STALE_AFTER", 5*time.Minute),
		ReclaimFailed:   envBool("WORM_RECLAIM_FAILED"),
		SkipMark:        envBool("WORM_SKIP_MARK"),
		MarkSummaryMax:  envInt("WORM_MARK_SUMMARY_MAX", 256),
		ExportSchemaVer: envOr("WORM_EXPORT_SCHEMA_VERSION", "v21.worm_export.1"),
	}

	// ---- ctx / shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigC := make(chan os.Signal, 2)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigC
		cancel()
	}()

	// ---- db
	db, err := postgres.New(ctx, pgDsn)
	if err != nil {
		log.Fatalf("[wormexportersvc] db connect failed: %v", err)
	}
	defer db.Close()

	// ---- repo
	repo := postgres.NewRepo(db)

	// ---- exporter (NOTE: DB is required to write export_result evidence via v18)
	exp, err := worm.NewExporter(repo, db, cfg)
	if err != nil {
		log.Fatalf("[wormexportersvc] init exporter failed: %v", err)
	}

	// ---- startup config log (debugging must be easy)
	log.Printf(
		"[wormexportersvc] boot project_id=%s sink=%s out=%s limit=%d once=%t every=%s stale_after=%s reclaim_failed=%t skip_mark=%t mark_summary_max=%d schema=%s",
		cfg.ProjectID,
		cfg.Sink,
		cfg.OutDir,
		cfg.Limit,
		cfg.RunOnce,
		cfg.Every,
		cfg.StaleAfter,
		cfg.ReclaimFailed,
		cfg.SkipMark,
		cfg.MarkSummaryMax,
		cfg.ExportSchemaVer,
	)

	// throw禁止: Runはエラーを返しても落とさず、ログして停止（または継続）する設計。
	// main 側は「終了理由」をログし、プロセスとしては正常終了でもよい。
	if err := exp.Run(ctx); err != nil {
		log.Printf("[wormexportersvc] run finished with err=%v", err)
	} else {
		log.Printf("[wormexportersvc] run finished ok")
	}
}