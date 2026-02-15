package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr string
	DB   int
}

func NewClient(cfg Config) *redis.Client {
	if cfg.Addr == "" {
		cfg.Addr = "ak_redis:6379"
	}
	return redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
		DB:   cfg.DB,
	})
}

func SetNX(ctx context.Context, rdb *redis.Client, key, val string, ttl time.Duration) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return rdb.SetNX(cctx, key, val, ttl).Result()
}

func Del(ctx context.Context, rdb *redis.Client, key string) error {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := rdb.Del(cctx, key).Result()
	return err
}