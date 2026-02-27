package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestV6UTL_IngestAndDuplicate(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var projectID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM public.projects ORDER BY id LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}

	var traceID string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&traceID); err != nil {
		t.Fatal(err)
	}

	provider := "stripe"
	providerEventID := "evt_test_dup_001"
	eventName := "payment_intent.succeeded"

	var id1 int64
	var st1, ek1, pk1 string
	if err := db.QueryRowContext(ctx, `
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
`, projectID, provider, providerEventID, eventName, traceID).Scan(&id1, &st1, &ek1, &pk1); err != nil {
		t.Fatal(err)
	}

	var id2 int64
	var st2, ek2, pk2 string
	if err := db.QueryRowContext(ctx, `
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
`, projectID, provider, providerEventID, eventName, traceID).Scan(&id2, &st2, &ek2, &pk2); err != nil {
		t.Fatal(err)
	}

	if ek1 != ek2 {
		t.Fatalf("event_key must match for duplicate ingest: %s != %s", ek1, ek2)
	}
	if id1 != id2 {
		t.Fatalf("utl_event_id should match for duplicate ingest: %d != %d", id1, id2)
	}
	if st2 != "duplicate" {
		t.Fatalf("second ingest should be duplicate, got %s", st2)
	}
	if len(pk1) != 64 {
		t.Fatalf("posting_key length expected 64, got %d", len(pk1))
	}
}
