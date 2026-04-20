package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

type Client = goredis.Client

// New builds a Redis client and pings it so startup fails fast on misconfig.
func New(ctx context.Context, addr string, db int) (*Client, error) {
	c := goredis.NewClient(&goredis.Options{Addr: addr, DB: db})
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping %s: %w", addr, err)
	}
	return c, nil
}
