package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestV7Identity_ResolveIsStable(t *testing.T) {
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
	poType := "payment_intent"
	poID := "pi_test_stable_001"

	var st1, it1, iid1 string
	var mid1 sql.NullInt64
	var cid1 sql.NullInt64

	err = db.QueryRowContext(ctx, `
SELECT out_status, out_internal_object_type, out_internal_object_id, out_mapping_id, out_conflict_id
FROM public.identity_resolve_v7(
  $1::varchar(26),
  $2::varchar,
  $3::varchar,
  $4::varchar,
  'payment'::varchar,
  true,
  $5::uuid,
  NULL::varchar,
  NULL::bigint
);
`, projectID, provider, poType, poID, traceID).Scan(&st1, &it1, &iid1, &mid1, &cid1)
	if err != nil {
		t.Fatal(err)
	}
	if st1 != "created" && st1 != "resolved" {
		t.Fatalf("expected created/resolved, got %s", st1)
	}

	var st2, it2, iid2 string
	var mid2 sql.NullInt64
	var cid2 sql.NullInt64
	err = db.QueryRowContext(ctx, `
SELECT out_status, out_internal_object_type, out_internal_object_id, out_mapping_id, out_conflict_id
FROM public.identity_resolve_v7(
  $1::varchar(26),
  $2::varchar,
  $3::varchar,
  $4::varchar,
  'payment'::varchar,
  true,
  $5::uuid,
  NULL::varchar,
  NULL::bigint
);
`, projectID, provider, poType, poID, traceID).Scan(&st2, &it2, &iid2, &mid2, &cid2)
	if err != nil {
		t.Fatal(err)
	}

	if it1 != it2 || iid1 != iid2 {
		t.Fatalf("resolve must be stable: (%s,%s) != (%s,%s)", it1, iid1, it2, iid2)
	}
	if cid2.Valid {
		t.Fatalf("expected no conflict in stable resolve")
	}
}