package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

type Client = goredis.Client

// New parses a Redis connection URL, builds a client, and pings it so startup
// fails fast on misconfig. The URL carries scheme (redis:// or rediss:// for
// TLS), optional username + password, host, port, and db index — see
// goredis.ParseURL for the full grammar.
func New(ctx context.Context, url string) (*Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	c := goredis.NewClient(opts)
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping %s: %w", opts.Addr, err)
	}
	return c, nil
}
