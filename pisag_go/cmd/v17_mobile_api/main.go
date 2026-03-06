package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/internal/adminapi"
	"example.com/pisag_go/postgres"
	"example.com/pisag_go/usecase"
)

func main() {
	ctx := context.Background()

	dsn := getenv("AK_PG_DSN", "postgres://ak:ak@127.0.0.1:5433/ak?sslmode=disable")
	addr := getenv("AK_HTTP_ADDR", ":8080")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}

	devices := postgres.NewMobileDeviceRepo(db)
	stepups := postgres.NewMobileStepUpRepo(db)
	inbox := postgres.NewMobileInboxRepo(db)
	receipts := postgres.NewMobileActionReceiptRepo(db)

	registerDeviceUC := &usecase.RegisterMobileDeviceUseCase{
		Devices: devices,
	}
	requestStepUpUC := &usecase.RequestMobileStepUpUseCase{
		Devices: devices,
		Inbox:   inbox,
		StepUps: stepups,
	}
	verifyStepUpUC := &usecase.VerifyMobileStepUpUseCase{
		Devices: devices,
		StepUps: stepups,
	}
	listInboxUC := &usecase.ListMobileInboxUseCase{
		Inbox: inbox,
	}
	ackItemUC := &usecase.AckMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}
	approveItemUC := &usecase.ApproveMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}
	rejectItemUC := &usecase.RejectMobileInboxItemUseCase{
		Devices:  devices,
		Inbox:    inbox,
		StepUps:  stepups,
		Receipts: receipts,
	}

	// B: dev seed endpoint 用
	seedInboxUC := &usecase.DevSeedMobileInboxUseCase{
		Inbox: inbox,
	}

	handler := &adminapi.V17MobileHandler{
		RegisterDeviceUC: registerDeviceUC,
		RequestStepUpUC:  requestStepUpUC,
		VerifyStepUpUC:   verifyStepUpUC,
		ListInboxUC:      listInboxUC,
		AckItemUC:        ackItemUC,
		ApproveItemUC:    approveItemUC,
		RejectItemUC:     rejectItemUC,
	}

	devHandler := &adminapi.V17MobileDevHandler{
		SeedInboxUC: seedInboxUC,
	}

	mux := http.NewServeMux()
	adminapi.RegisterV17MobileRoutes(mux, handler, devHandler)

	log.Printf("v17 mobile api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http serve: %v", err)
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}
