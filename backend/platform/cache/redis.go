package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisCache is the production Cache implementation. It piggy-backs on the
// shared *goredis.Client built by common/redis.New — never construct a fresh
// connection here.
type RedisCache struct {
	rdb *goredis.Client
}

// NewRedisCache wraps the given redis client. Caller retains ownership of the
// client and is responsible for closing it.
func NewRedisCache(rdb *goredis.Client) *RedisCache {
	return &RedisCache{rdb: rdb}
}

// Get returns (false, nil) on a clean miss. Transport / decode failures are
// returned to the caller so cache decorators can decide whether to fall
// through to the source of truth.
func (c *RedisCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("cache redis get %q: %w", key, err)
	}
	if err := unmarshal(b, dest); err != nil {
		return false, err
	}
	return true, nil
}

// Set stores the JSON encoding of val under key with ttl. ttl <= 0 means
// "no expiry" (Redis default).
func (c *RedisCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	data, err := marshal(val)
	if err != nil {
		return err
	}
	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache redis set %q: %w", key, err)
	}
	return nil
}

// Delete removes the listed keys; missing keys are ignored.
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache redis del: %w", err)
	}
	return nil
}
