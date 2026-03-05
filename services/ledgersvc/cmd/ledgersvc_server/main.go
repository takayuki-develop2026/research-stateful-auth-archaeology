package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httpapi "ledgersvc/http"
	"ledgersvc/postgres"
)

func main() {
	addr := getenvDefault("LEDGERSVC_ADDR", ":9031")
	dsn := mustEnv("PG_DSN")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	dieIf(err, "pgxpool.New failed")
	defer pool.Close()

	ingRead := postgres.NewLedgerIngestRunReadRepo(pool)
	evRead := postgres.NewEvidenceReadRepo(pool)
	h := httpapi.NewHandler(ingRead, evRead)

	s := &httpapi.Server{
		Addr:    addr,
		Handler: h,
	}

	fmt.Printf("[ledgersvc_server] listening on %s\n", addr)
	dieIf(s.ListenAndServe(), "ListenAndServe failed")
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing env: %s\n", k)
		os.Exit(2)
	}
	return v
}
func getenvDefault(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func dieIf(err error, msg string) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", msg, err)
	os.Exit(1)
}