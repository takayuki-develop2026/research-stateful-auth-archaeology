package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func openDBOrSkip(t *testing.T, env string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skip(env + " が未設定のためスキップします")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("DB接続失敗: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("DB ping失敗: %v", err)
	}
	return db
}

func TestStartFetchRun_DB_RunReuse(t *testing.T) {
	db := openDBOrSkip(t, "DATABASE_URL") // owner/管理ユーザー想定
	defer db.Close()

	rr := postgres.NewRunRepository(db)
	ir := postgres.NewRunInputRepository(db)
	er := postgres.NewRunEventRepository(db)

	uc := usecase.StartFetchRunUseCase{
		Fetcher: fakeFetcher{res: usecase.FetchResult{
			FinalURL:    "https://example.com/real-test",
			StatusCode:  200,
			ContentType: "text/html",
			BodySize:    500,
		}},
		RunRepo:      rr,
		RunInputRepo: ir,
		RunEventRepo: er,
	}

	ctx := context.Background()

	allowKey := "oracle" // ✅ v4 fixed: allowlist_key is required (fail-closed)

	in := usecase.StartFetchRunInput{
		ProjectID:       "integration-test-project",
		TargetURL:       "https://oracle.singularity.local/pricing_v1.json",
		PipelineVersion: "v4.1-test",
		AllowlistKey:    &allowKey, // ✅ 必須
		ImmediateFetch:  false,     // worker主体想定：enqueueだけ
		ReuseRun:        nil,       // ✅ デフォルトtrue
	}

	out1, err := uc.Handle(ctx, in)
	if err != nil {
		t.Fatalf("1st Handle failed: %v", err)
	}
	out2, err := uc.Handle(ctx, in)
	if err != nil {
		t.Fatalf("2nd Handle failed: %v", err)
	}

	// ✅ run reuse（同一run_id）
	if out1.RunID != out2.RunID {
		t.Fatalf("expected reuse same run_id, got %s vs %s", out1.RunID, out2.RunID)
	}

	// ✅ enqueue idempotency（run_inputs が増えていない）
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM run_inputs WHERE run_id=$1`, out1.RunID).Scan(&cnt); err != nil {
		t.Fatalf("count run_inputs failed: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected run_inputs=1, got %d", cnt)
	}

	t.Logf("✅ reuse OK: run_id=%s inputs=%d allowlist_key=%s", out1.RunID, cnt, allowKey)
}