package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/httpx"
	"example.com/pisag_go/internal/adminapi"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	addr := getenv("AK_ADMIN_API_ADDR", ":8082")

	// 既存 admin/discovery-ops 側は database/sql を継続利用
	sqlDSN := getenv("DATABASE_URL", "postgresql://ak@localhost:5433/ak")

	// v17 は pgxpool ベース repo を使うので別で DSN を取る
	// 未指定なら DATABASE_URL を流用
	pgxDSN := getenv("AK_PG_DSN", sqlDSN)

	// ---------------------------------------------------------
	// Existing admin API DB (database/sql)
	// ---------------------------------------------------------
	db, err := sql.Open("pgx", sqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	// ---------------------------------------------------------
	// v17 Mobile Integration DB (pgxpool)
	// ---------------------------------------------------------
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, pgxDSN)
	if err != nil {
		log.Fatalf("pgxpool connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("pgxpool ping: %v", err)
	}

	// ---------------------------------------------------------
	// Existing discovery ops server
	// ---------------------------------------------------------
	srv := &adminapi.Server{
		Ops: postgres.NewDiscoveryOpsRepo(db),
	}

	// ---------------------------------------------------------
	// v17 Mobile Integration wiring
	// ---------------------------------------------------------
	v17Devices := postgres.NewMobileDeviceRepo(pool)
	v17StepUps := postgres.NewMobileStepUpRepo(pool)
	v17Inbox := postgres.NewMobileInboxRepo(pool)
	v17Receipts := postgres.NewMobileActionReceiptRepo(pool)

	v17RegisterDeviceUC := &usecase.RegisterMobileDeviceUseCase{
		Devices: v17Devices,
	}
	v17RequestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: v17Devices,
		Inbox:   v17Inbox,
		StepUps: v17StepUps,
	}
	v17VerifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: v17Devices,
		StepUps: v17StepUps,
	}
	v17ListInboxUC := &usecase.ListMobileInboxUseCase{
		Inbox: v17Inbox,
	}
	v17AckItemUC := &usecase.AckMobileInboxItemUseCase{
		Devices:  v17Devices,
		Inbox:    v17Inbox,
		StepUps:  v17StepUps,
		Receipts: v17Receipts,
	}
	v17ApproveItemUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  v17Devices,
		Inbox:    v17Inbox,
		StepUps:  v17StepUps,
		Receipts: v17Receipts,
	}
	v17RejectItemUC := &usecase.RejectMobileInboxItemUseCase{
		Devices:  v17Devices,
		Inbox:    v17Inbox,
		StepUps:  v17StepUps,
		Receipts: v17Receipts,
	}

	v17Handler := &adminapi.V17MobileHandler{
		RegisterDeviceUC: v17RegisterDeviceUC,
		RequestStepUpUC:  v17RequestStepUpUC,
		VerifyStepUpUC:   v17VerifyStepUpUC,
		ListInboxUC:      v17ListInboxUC,
		AckItemUC:        v17AckItemUC,
		ApproveItemUC:    v17ApproveItemUC,
		RejectItemUC:     v17RejectItemUC,
	}

	// dev seed endpoint は開発中だけ有効化できるようにする
	enableV17DevSeed := getenv("AK_ENABLE_V17_DEV_SEED", "true") == "true"

	var v17DevHandler *adminapi.V17MobileDevHandler
	if enableV17DevSeed {
		v17SeedInboxUC := &usecase.DevSeedMobileInboxUseCase{
			Inbox: v17Inbox,
		}
		v17DevHandler = &adminapi.V17MobileDevHandler{
			SeedInboxUC: v17SeedInboxUC,
		}
	}

	// ---------------------------------------------------------
	// Routes
	// ---------------------------------------------------------
	mux := http.NewServeMux()

	// Existing admin routes
	mux.Handle("/api/admin/atlaskernel/discovery-ops/", srv)
	mux.Handle("/api/admin/atlaskernel/discovery-ops", srv)

	// v17 routes
	adminapi.RegisterV17MobileRoutes(mux, v17Handler, v17DevHandler)

	log.Printf("[ak_admin_api] listen %s", addr)
	if err := http.ListenAndServe(addr, httpx.TraceMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
