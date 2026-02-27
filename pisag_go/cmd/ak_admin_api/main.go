package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"example.com/pisag_go/httpx"
	"example.com/pisag_go/internal/adminapi"
	"example.com/pisag_go/postgres"
)

func main() {
	addr := getenv("AK_ADMIN_API_ADDR", ":8082")
	dsn := getenv("DATABASE_URL", "postgresql://ak@localhost:5433/ak")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	srv := &adminapi.Server{
		Ops: postgres.NewDiscoveryOpsRepo(db),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/admin/atlaskernel/discovery-ops/", srv)
	mux.Handle("/api/admin/atlaskernel/discovery-ops", srv)

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
