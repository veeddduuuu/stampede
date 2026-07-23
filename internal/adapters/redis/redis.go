package redis

import (
	"context"
	"log"

	goredis "github.com/redis/go-redis/v9"
)

func NewRedisClient(addr string) *goredis.Client {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:         addr,
		PoolSize:     100,
		MinIdleConns: 20,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis: %s", addr)
	return rdb
}
