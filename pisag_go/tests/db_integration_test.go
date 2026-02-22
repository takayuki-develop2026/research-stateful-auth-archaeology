package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
	_ "github.com/jackc/pgx/v5/stdlib" // pgxドライバをロード
)

func TestStartFetchRun_WithRealDB(t *testing.T) {
	// 1. 環境変数から接続情報を取得（ak_worker権限を想定）
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL が設定されていないため統合テストをスキップします")
	}

	// 2. 本物のDBに接続
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("DB接続失敗: %v", err)
	}
	defer db.Close()

	// 3. 本物のリポジトリをインスタンス化
	rr := postgres.NewRunRepository(db)
	ir := postgres.NewRunInputRepository(db)
	er := postgres.NewRunEventRepository(db)

	// 4. UseCaseのセットアップ（Fetcherだけは外部通信を避けるためモック）
	uc := usecase.StartFetchRunUseCase{
		Fetcher: fakeFetcher{res: usecase.FetchResult{
			FinalURL:    "http://example.com/real-test",
			StatusCode:  200,
			ContentType: "text/html",
			BodySize:    500,
		}},
		RunRepo:      rr,
		RunInputRepo: ir,
		RunEventRepo: er,
	}

	// 5. 実行
	ctx := context.Background()
	out, err := uc.Handle(ctx, usecase.StartFetchRunInput{
		ProjectID:       "integration-test-project",
		TargetURL:       "https://example.com",
		PipelineVersion: "v4.1-test",
	})

	// 6. 検証
	if err != nil {
		t.Fatalf("UseCaseの実行に失敗しました（権限不足の可能性あり）: %v", err)
	}

	if out.RunID == "" {
		t.Error("RunID が生成されていません")
	}

	t.Logf("✅ 統合テスト成功: RunID=%s としてDBに記録されました", out.RunID)

	// 🛡️ 「世界プロ」の権限チェック：SELECTを試みて「拒否」されるか確認
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM runs").Scan(&count)
	if err == nil {
		t.Error("🛡️ 警告: ak_worker なのに SELECT に成功しました。権限設定が漏れています！")
	} else {
		t.Logf("🛡️ 防御成功: SELECT は期待通り拒否されました: %v", err)
	}
}