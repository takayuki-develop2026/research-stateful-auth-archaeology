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

	dbURL := mustEnv("DATABASE_URL")
	db, err := postgres.Open(dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectID := firstNonEmpty(os.Getenv("PROJECT_ID"), mustQueryString(ctx, db, `SELECT id FROM public.projects ORDER BY id LIMIT 1`))
	traceID := mustQueryString(ctx, db, `SELECT gen_random_uuid()::text`)

	provider := "stripe"
	poType := "payment_intent"
	poID := "pi_smoke_001"

	// resolve create
	st1, it1, iid1, mid1, cid1 := resolve(ctx, db, projectID, provider, poType, poID, "payment", true, traceID, "", 0)
	log.Printf("[v7_smoke] resolve #1 status=%s type=%s id=%s mapping_id=%d conflict_id=%v", st1, it1, iid1, mid1, cid1)

	// resolve again (must resolve same)
	st2, it2, iid2, mid2, cid2 := resolve(ctx, db, projectID, provider, poType, poID, "payment", true, traceID, "", 0)
	log.Printf("[v7_smoke] resolve #2 status=%s type=%s id=%s mapping_id=%d conflict_id=%v", st2, it2, iid2, mid2, cid2)

	// assign conflict attempt (set_active with different internal id)
	otherInternal := "pay_" + strings.ReplaceAll(mustQueryString(ctx, db, `SELECT gen_random_uuid()::text`), "-", "")
	as1, am1, ac1 := assign(ctx, db, projectID, provider, poType, poID, "payment", otherInternal, "set_active", traceID)
	log.Printf("[v7_smoke] assign set_active status=%s mapping_id=%v conflict_id=%v", as1, am1, ac1)

	// supersede_active (should succeed)
	as2, am2, ac2 := assign(ctx, db, projectID, provider, poType, poID, "payment", otherInternal, "supersede_active", traceID)
	log.Printf("[v7_smoke] assign supersede status=%s mapping_id=%v conflict_id=%v", as2, am2, ac2)

	log.Printf("[v7_smoke] ✅ DONE")
}

func resolve(ctx context.Context, db *sql.DB, projectID, provider, poType, poID, internalType string, create bool, traceID, eventKeyRef string, payloadEvidenceID int64) (string, string, string, int64, *int64) {
	var st, it, iid string
	var mid sql.NullInt64
	var cid sql.NullInt64

	err := db.QueryRowContext(ctx, `
SELECT out_status, out_internal_object_type, out_internal_object_id, out_mapping_id, out_conflict_id
FROM public.identity_resolve_v7(
  $1::varchar(26),
  $2::varchar,
  $3::varchar,
  $4::varchar,
  $5::varchar,
  $6::boolean,
  $7::uuid,
  NULLIF($8,'')::varchar,
  NULLIF($9::bigint,0)
);
`, projectID, provider, poType, poID, internalType, create, traceID, eventKeyRef, payloadEvidenceID).Scan(&st, &it, &iid, &mid, &cid)
	if err != nil {
		log.Fatalf("resolve failed: %v", err)
	}

	mappingID := int64(0)
	if mid.Valid {
		mappingID = mid.Int64
	}
	var conflictID *int64 = nil
	if cid.Valid {
		v := cid.Int64
		conflictID = &v
	}
	return st, it, iid, mappingID, conflictID
}

func assign(ctx context.Context, db *sql.DB, projectID, provider, poType, poID, internalType, internalID, mode, traceID string) (string, *int64, *int64) {
	var st string
	var mid sql.NullInt64
	var cid sql.NullInt64

	err := db.QueryRowContext(ctx, `
SELECT out_status, out_mapping_id, out_conflict_id
FROM public.identity_assign_v7(
  $1::varchar(26),
  $2::varchar,
  $3::varchar,
  $4::varchar,
  $5::varchar,
  $6::varchar,
  $7::varchar,
  $8::uuid,
  NULL::varchar,
  NULL::bigint
);
`, projectID, provider, poType, poID, internalType, internalID, mode, traceID).Scan(&st, &mid, &cid)
	if err != nil {
		log.Fatalf("assign failed: %v", err)
	}

	var mappingID *int64 = nil
	if mid.Valid {
		v := mid.Int64
		mappingID = &v
	}
	var conflictID *int64 = nil
	if cid.Valid {
		v := cid.Int64
		conflictID = &v
	}
	return st, mappingID, conflictID
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
