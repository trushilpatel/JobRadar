package main

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
)

// TestBuildMultiUserPlatform_RedisUnreachable_FallsBackToMemory drives the
// fallback branch: an unreachable Redis URL makes redis.New ping-fail, so the
// builder must return the in-memory cache + limiter pair.
func TestBuildMultiUserPlatform_RedisUnreachable_FallsBackToMemory(t *testing.T) {
	cfg := &config.Config{}
	// 127.0.0.1:1 is reserved/unbindable — the ping fails fast.
	cfg.Redis.URL = "redis://127.0.0.1:1"
	cfg.MultiUser.RateLimitWriteRPM = 60

	c, lim := buildMultiUserPlatform(cfg, logger.NewNoop())

	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("cache type = %T, want *cache.MemoryCache on Redis failure", c)
	}
	if _, ok := lim.(*ratelimit.MemoryLimiter); !ok {
		t.Fatalf("limiter type = %T, want *ratelimit.MemoryLimiter on Redis failure", lim)
	}
}

// TestBuildMultiUserPlatform_RedisUnreachable_DefaultBurst covers the burst<=0
// default branch (RateLimitWriteRPM unset) on the in-memory fallback path.
func TestBuildMultiUserPlatform_RedisUnreachable_DefaultBurst(t *testing.T) {
	cfg := &config.Config{}
	cfg.Redis.URL = "redis://127.0.0.1:1"
	cfg.MultiUser.RateLimitWriteRPM = 0 // forces the burst = 20 default

	c, lim := buildMultiUserPlatform(cfg, logger.NewNoop())

	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("cache type = %T, want *cache.MemoryCache", c)
	}
	if _, ok := lim.(*ratelimit.MemoryLimiter); !ok {
		t.Fatalf("limiter type = %T, want *ratelimit.MemoryLimiter", lim)
	}
}

// TestBuildMultiUserPlatform_RedisURLParseError covers the case where the URL
// is malformed: redis.New returns a parse error (no network needed) and the
// builder still falls back to the in-memory pair.
func TestBuildMultiUserPlatform_RedisURLParseError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Redis.URL = "not-a-valid-redis-url"
	cfg.MultiUser.RateLimitWriteRPM = 30

	c, lim := buildMultiUserPlatform(cfg, logger.NewNoop())

	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("cache type = %T, want *cache.MemoryCache on parse error", c)
	}
	if _, ok := lim.(*ratelimit.MemoryLimiter); !ok {
		t.Fatalf("limiter type = %T, want *ratelimit.MemoryLimiter on parse error", lim)
	}
}

// TestBuildMultiUserPlatform_RedisConnected_LocalCacheWrap drives the success
// branch against an in-process miniredis. With a positive local TTL + max
// entries, the cache is the bounded LocalCache wrapper and the limiter is the
// Redis-backed limiter.
func TestBuildMultiUserPlatform_RedisConnected_LocalCacheWrap(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	cfg := &config.Config{}
	cfg.Redis.URL = "redis://" + mr.Addr()
	cfg.MultiUser.RateLimitWriteRPM = 120
	cfg.MultiUser.IdentityCacheLocalTTLSeconds = 30
	cfg.MultiUser.IdentityCacheLocalMaxEntries = 1000

	c, lim := buildMultiUserPlatform(cfg, logger.NewNoop())

	if _, ok := c.(*cache.LocalCache); !ok {
		t.Fatalf("cache type = %T, want *cache.LocalCache when local layer enabled", c)
	}
	if _, ok := lim.(*ratelimit.RedisLimiter); !ok {
		t.Fatalf("limiter type = %T, want *ratelimit.RedisLimiter when Redis reachable", lim)
	}
}

// TestBuildMultiUserPlatform_RedisConnected_LocalLayerDisabled covers the
// success branch where the local TTL/size are non-positive: NewLocalCache then
// returns the bare RedisCache (no wrapper). Also exercises the burst<=0 default
// inside the Redis branch.
func TestBuildMultiUserPlatform_RedisConnected_LocalLayerDisabled(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	cfg := &config.Config{}
	cfg.Redis.URL = "redis://" + mr.Addr()
	cfg.MultiUser.RateLimitWriteRPM = 0 // burst = 20 default
	cfg.MultiUser.IdentityCacheLocalTTLSeconds = 0
	cfg.MultiUser.IdentityCacheLocalMaxEntries = 0

	c, lim := buildMultiUserPlatform(cfg, logger.NewNoop())

	if _, ok := c.(*cache.RedisCache); !ok {
		t.Fatalf("cache type = %T, want *cache.RedisCache when local layer disabled", c)
	}
	if _, ok := lim.(*ratelimit.RedisLimiter); !ok {
		t.Fatalf("limiter type = %T, want *ratelimit.RedisLimiter", lim)
	}
}
