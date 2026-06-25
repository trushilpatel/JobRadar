package main

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	commonredis "github.com/trushilpatel/jobradar/common/redis"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
)

const redisConnectTimeout = 2 * time.Second

// buildMultiUserPlatform constructs the cache + rate-limiter pair used by
// the Phase 1 multi-user middleware. Falls back to in-memory implementations
// when Redis is unreachable — the goal is that a developer can boot the API
// without standing up Redis first.
func buildMultiUserPlatform(cfg *config.Config, log logger.Logger) (cache.Cache, ratelimit.Limiter) {
	ctx, cancel := context.WithTimeout(context.Background(), redisConnectTimeout)
	defer cancel()

	rdb, err := commonredis.New(ctx, cfg.Redis.URL)
	if err != nil {
		log.Warn("redis unavailable; using in-memory cache + limiter", logger.Err(err))
		burst := float64(cfg.MultiUser.RateLimitWriteRPM)
		if burst <= 0 {
			burst = 20
		}
		refill := burst / 60.0
		return cache.NewMemoryCache(), ratelimit.NewMemoryLimiter(burst, refill)
	}

	// Wrap Redis in a bounded in-process LRU. The same instance is fanned
	// out to the identity middleware and every service-layer cache decorator,
	// so any Set/Delete performed on this Cache clears the local layer as a
	// side effect — the user service does not need to know it exists.
	c := cache.NewLocalCache(
		cache.NewRedisCache(rdb),
		time.Duration(cfg.MultiUser.IdentityCacheLocalTTLSeconds)*time.Second,
		cfg.MultiUser.IdentityCacheLocalMaxEntries,
	)
	// Use the write RPM as the conservative budget — read RPM is a higher
	// limit but the limiter middleware picks the scope by HTTP method so
	// either RPM is honoured for its scope.
	burst := float64(cfg.MultiUser.RateLimitWriteRPM)
	if burst <= 0 {
		burst = 20
	}
	refill := burst / 60.0
	limiter := ratelimit.NewRedisLimiter(rdb, burst, refill, "ratelimit")
	return c, limiter
}
