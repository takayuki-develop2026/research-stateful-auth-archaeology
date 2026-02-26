package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/postgres"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dsn := mustEnv("DATABASE_URL")
	db, err := postgres.Open(dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectID := firstNonEmpty(os.Getenv("PROJECT_ID"), mustQueryString(ctx, db, `SELECT id FROM public.projects ORDER BY id LIMIT 1`))

	traceID := mustQueryString(ctx, db, `SELECT gen_random_uuid()::text`)

	// ---- webhook ingest (same provider_event_id twice => duplicate)
	provider := "stripe"
	providerEventID := "evt_smoke_001"
	eventName := "payment_intent.succeeded"

	utlID1, st1, ek1, pk1 := ingestWebhook(ctx, db, projectID, traceID, provider, providerEventID, eventName)
	utlID2, st2, ek2, pk2 := ingestWebhook(ctx, db, projectID, traceID, provider, providerEventID, eventName)

	log.Printf("[v6_smoke] webhook #1 id=%d status=%s event_key=%s posting_key=%s", utlID1, st1, ek1, pk1)
	log.Printf("[v6_smoke] webhook #2 id=%d status=%s event_key=%s posting_key=%s", utlID2, st2, ek2, pk2)

	// ---- internal ingest (same correlation_id + seq => duplicate)
	correlationID := "run_" + mustQueryString(ctx, db, `SELECT gen_random_uuid()::text`)
	internalName := "utl.ingested"
	utlID3, st3, ek3, pk3 := ingestInternal(ctx, db, projectID, traceID, correlationID, 10, internalName)
	utlID4, st4, ek4, pk4 := ingestInternal(ctx, db, projectID, traceID, correlationID, 10, internalName)

	log.Printf("[v6_smoke] internal #1 id=%d status=%s event_key=%s posting_key=%s", utlID3, st3, ek3, pk3)
	log.Printf("[v6_smoke] internal #2 id=%d status=%s event_key=%s posting_key=%s", utlID4, st4, ek4, pk4)

	// ---- query
	rowCount := mustQueryInt64(ctx, db, `SELECT count(*) FROM public.universal_events_v6 WHERE project_id=$1`, projectID)
	log.Printf("[v6_smoke] events count=%d", rowCount)

	// ---- replay request
	replayStatus := requestReplay(ctx, db, projectID, traceID, ek1)
	log.Printf("[v6_smoke] replay status=%s for event_key=%s", replayStatus, ek1)

	log.Printf("[v6_smoke] ✅ DONE")
}

func ingestWebhook(ctx context.Context, db *sql.DB, projectID, traceID, provider, providerEventID, eventName string) (int64, string, string, string) {
	var id int64
	var st, ek, pk string

	err := db.QueryRowContext(ctx, `
SELECT out_utl_event_id, out_status, out_event_key, out_posting_key
FROM public.utl_ingest_v6(
  $1::varchar(26),
  'webhook'::varchar,
  $2::varchar,
  $3::varchar,
  $4::varchar,
  now()::timestamptz,
  now()::timestamptz,
  NULL::varchar,
  NULL::int,
  $5::uuid,
  NULL::uuid,
  NULL::bigint,
  NULL::char(3),
  NULL::varchar,
  NULL::varchar,
  NULL::varchar,
  NULL::bigint
);
`, projectID, provider, providerEventID, eventName, traceID).Scan(&id, &st, &ek, &pk)
	if err != nil {
		log.Fatalf("ingestWebhook failed: %v", err)
	}
	return id, st, ek, pk
}

func ingestInternal(ctx context.Context, db *sql.DB, projectID, traceID, correlationID string, seq int, eventName string) (int64, string, string, string) {
	var id int64
	var st, ek, pk string

	err := db.QueryRowContext(ctx, `
SELECT out_utl_event_id, out_status, out_event_key, out_posting_key
FROM public.utl_ingest_v6(
  $1::varchar(26),
  'internal'::varchar,
  'internal'::varchar,
  NULL::varchar,
  $2::varchar,
  now()::timestamptz,
  now()::timestamptz,
  $3::varchar,
  $4::int,
  $5::uuid,
  NULL::uuid,
  NULL::bigint,
  NULL::char(3),
  NULL::varchar,
  NULL::varchar,
  NULL::varchar,
  NULL::bigint
);
`, projectID, eventName, correlationID, seq, traceID).Scan(&id, &st, &ek, &pk)
	if err != nil {
		log.Fatalf("ingestInternal failed: %v", err)
	}
	return id, st, ek, pk
}

func requestReplay(ctx context.Context, db *sql.DB, projectID, traceID, eventKey string) string {
	var st string
	err := db.QueryRowContext(ctx, `
SELECT status
FROM public.utl_request_replay_v6($1, $2, $3::uuid, NULL);
`, projectID, eventKey, traceID).Scan(&st)
	if err != nil {
		log.Fatalf("requestReplay failed: %v", err)
	}
	return st
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}
func firstNonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		return v
	}
	return fallback
}
func mustQueryString(ctx context.Context, db *sql.DB, q string, args ...any) string {
	var s string
	if err := db.QueryRowContext(ctx, q, args...).Scan(&s); err != nil {
		log.Fatalf("query string failed: %v", err)
	}
	return strings.TrimSpace(s)
}
func mustQueryInt64(ctx context.Context, db *sql.DB, q string, args ...any) int64 {
	var n int64
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		log.Fatalf("query int64 failed: %v", err)
	}
	return n
}