package postgres

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context) (*DB, error) {
	dsn := os.Getenv("AK_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("RUNSCHED_PG_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// sensible defaults
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}