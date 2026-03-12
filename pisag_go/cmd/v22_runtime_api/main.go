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
	addr := getenv("AK_RUNTIME_API_ADDR", ":9082")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}

	// ---------------------------------
	// Repositories
	// ---------------------------------
	taskRepo := postgres.NewMultimodalTaskRepo(db)
	taskInputRepo := postgres.NewMultimodalTaskInputRepo(db)
	runtimeDetailRepo := postgres.NewRuntimeRunDetailRepo(db)
	runtimeRequestEvidenceRepo := &postgres.RuntimeRequestEvidenceRepo{DB: db}

	runtimeUploadEvidenceRepo := postgres.NewRuntimeUploadEvidenceRepo(db)

	// ---------------------------------
	// UseCases
	// ---------------------------------
	routeUC := &usecase.RouteV22ModelTaskUseCase{}

	registerEvidenceUC := &usecase.RegisterRuntimeRequestEvidenceUseCase{
		Evidence: runtimeRequestEvidenceRepo,
	}

	createTaskUC := &usecase.CreateMultimodalTaskUseCase{
		Tasks: taskRepo,
	}

	attachInputsUC := &usecase.AttachMultimodalTaskInputsUseCase{
		Tasks:      taskRepo,
		TaskInputs: taskInputRepo,
	}

	getDetailUC := &usecase.GetRuntimeRunDetailUseCase{
		Details: runtimeDetailRepo,
	}

	registerUploadedEvidenceUC := &usecase.RegisterRuntimeUploadedEvidenceUseCase{
		Evidence: runtimeUploadEvidenceRepo,
	}

	getUploadedEvidenceSummaryUC := &usecase.GetRuntimeUploadedEvidenceSummaryUseCase{
		Evidence: runtimeUploadEvidenceRepo,
	}

	// ---------------------------------
	// HTTP Handler
	// ---------------------------------
	handler := &adminapi.V22RuntimeHandler{
		RouteTask:                  routeUC,
		RegisterRequestEvidence:    registerEvidenceUC,
		CreateTask:                 createTaskUC,
		AttachTaskInputs:           attachInputsUC,
		GetRunDetail:               getDetailUC,
		RegisterUploadedEvidence:   registerUploadedEvidenceUC,
		GetUploadedEvidenceSummary: getUploadedEvidenceSummaryUC,
	}

	mux := http.NewServeMux()
	handler.Register(mux)

	log.Printf("v22 runtime api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}