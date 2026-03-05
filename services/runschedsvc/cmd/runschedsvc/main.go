package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"runschedsvc/internal/app"
	"runschedsvc/postgres"
)

func envOr(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDur(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	projectID := envOr("AK_PROJECT_ID", "demo")

	db, err := postgres.Open(ctx)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	cfg := app.Config{
		ProjectID:        projectID,
		TickEvery:        envDur("RUNSCHED_TICK_EVERY", 60*time.Second),
		DispatchEvery:    envDur("RUNSCHED_DISPATCH_EVERY", 2*time.Second),
		TickLimit:        envInt("RUNSCHED_TICK_LIMIT", 50),
		DispatchLimit:    envInt("RUNSCHED_DISPATCH_LIMIT", 200),
		CronPreviewLimit: envInt("RUNSCHED_CRON_PREVIEW_LIMIT", 200),
	}

	a := app.New(db, cfg)

	// graceful shutdown
	sigC := make(chan os.Signal, 2)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigC
		cancel()
	}()

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("app run: %v", err)
	}
}