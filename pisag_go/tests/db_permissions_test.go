package tests

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDB_Permissions_DenySelect(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_WORKER")
	if dsn == "" {
		t.Skip("DATABASE_URL_WORKER が未設定のためスキップします")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("DB接続失敗: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("DB ping失敗: %v", err)
	}

	// ✅ SELECT が拒否されることだけ確認（write-only の最小テスト）
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM runs").Scan(&count)
	if err == nil {
		t.Fatalf("🛡️ 失敗: ak_worker が SELECT に成功しました。権限設定漏れです（count=%d）", count)
	}

	t.Logf("🛡️ 防御成功: SELECT は期待通り拒否されました: %v", err)
}