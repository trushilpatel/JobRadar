package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed record_failure.lua
var recordFailureLua string

var recordFailureScript = goredis.NewScript(recordFailureLua)

type cooldownRedisStore struct {
	rdb    *goredis.Client
	policy CooldownPolicy

	mu   sync.Mutex
	rand *rand.Rand
}

// NewLLMCooldownRedisStore returns a Redis-backed LLMCooldownStore.
// Keys:
//
//	cooldown:{slotID}:failure_count  INT
//	cooldown:{slotID}:cooldown_until INT (unix millis)
func NewLLMCooldownRedisStore(rdb *goredis.Client, p CooldownPolicy) LLMCooldownStore {
	return &cooldownRedisStore{
		rdb:    rdb,
		policy: p,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *cooldownRedisStore) countKey(id string) string {
	return fmt.Sprintf("cooldown:%s:failure_count", id)
}

func (c *cooldownRedisStore) untilKey(id string) string {
	return fmt.Sprintf("cooldown:%s:cooldown_until", id)
}

func (c *cooldownRedisStore) IsAvailable(ctx context.Context, slotID string) (bool, error) {
	until, err := c.rdb.Get(ctx, c.untilKey(slotID)).Int64()
	if errors.Is(err, goredis.Nil) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("cooldown redis get until: %w", err)
	}
	return time.Now().UnixMilli() > until, nil
}

func (c *cooldownRedisStore) RecordSuccess(ctx context.Context, slotID string) error {
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, c.untilKey(slotID))
	pipe.Set(ctx, c.countKey(slotID), 0, time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("cooldown redis record success: %w", err)
	}
	return nil
}

func (c *cooldownRedisStore) RecordFailure(ctx context.Context, slotID string) error {
	// Pre-compute jitter multiplier in Go so tests remain deterministic when
	// policy.Jitter == 0. Mirrors computeBackoff: multiplier = 1 + (roll*2-1)*Jitter.
	multiplier := 1.0
	if c.policy.Jitter > 0 {
		c.mu.Lock()
		roll := c.rand.Float64()
		c.mu.Unlock()
		multiplier = 1 + (roll*2-1)*c.policy.Jitter
	}

	now := time.Now()
	// Keep the counter alive long enough to outlast the cooldown plus grace.
	countExpireAt := now.Add(c.policy.Max + time.Hour).UnixMilli()

	keys := []string{c.countKey(slotID), c.untilKey(slotID)}
	args := []any{
		c.policy.Base.Milliseconds(),
		c.policy.Max.Milliseconds(),
		multiplier,
		now.UnixMilli(),
		countExpireAt,
	}
	if _, err := recordFailureScript.Run(ctx, c.rdb, keys, args...).Int64(); err != nil {
		return fmt.Errorf("cooldown redis record failure: %w", err)
	}
	return nil
}

func (c *cooldownRedisStore) Status(ctx context.Context, slotID string) (CooldownStatus, error) {
	pipe := c.rdb.Pipeline()
	countCmd := pipe.Get(ctx, c.countKey(slotID))
	untilCmd := pipe.Get(ctx, c.untilKey(slotID))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return CooldownStatus{}, fmt.Errorf("cooldown redis status: %w", err)
	}
	var s CooldownStatus
	if v, err := countCmd.Int(); err == nil {
		s.FailureCount = v
	}
	if v, err := untilCmd.Int64(); err == nil {
		s.CooldownUntil = time.UnixMilli(v)
	}
	return s, nil
}
