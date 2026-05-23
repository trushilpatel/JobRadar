package store

import (
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/common/config"
)

// NewRedisStores creates the rate-limit and cooldown stores from a Redis client and cooldown config.
func NewRedisStores(rdb *goredis.Client, cfg config.CooldownConfig) (LLMRateLimitStore, LLMCooldownStore) {
	rateStore := NewLLMRateLimitRedisStore(rdb)
	cooldownStore := NewLLMCooldownRedisStore(rdb, CooldownPolicy{
		Base:   time.Duration(cfg.BaseSeconds) * time.Second,
		Max:    time.Duration(cfg.MaxSeconds) * time.Second,
		Jitter: cfg.Jitter,
	})
	return rateStore, cooldownStore
}
