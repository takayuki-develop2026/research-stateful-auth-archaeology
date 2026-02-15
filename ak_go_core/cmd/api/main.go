package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/ak_go_core/internal/httpx"
)

func main() {
	// ----------------------------
	// DB init (v3 SoT)
	// ----------------------------
	dsn := os.Getenv("AK_DB_DSN")
	if dsn == "" {
		log.Fatal("AK_DB_DSN is required")
	}

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db init error: %v", err)
	}
	defer db.Close()

	// ★追加: 起動時に必ず疎通確認して、原因をここで落とす
	{
		// 3秒以内にDBと繋がらなければエラーにしてサーバーを起動させない
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			log.Fatalf("db ping error (疎通失敗): %v", err)
		}
		log.Println("db connection check: OK")
	}

	// ----------------------------
	// Server Setup
	// ----------------------------
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	addr := ":" + port

	router := httpx.NewRouter(db)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("AK Go Core listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-done
	log.Printf("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Printf("bye")
}
