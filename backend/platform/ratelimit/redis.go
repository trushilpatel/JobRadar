package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisLimiter is the production token-bucket Limiter. State is shared
// across API replicas via Redis; a single Lua script performs the read,
// refill, debit, and write atomically so concurrent replicas cannot
// double-spend a bucket.
//
// Reuses the *goredis.Client built by common/redis.New — never opens a
// fresh connection.
type RedisLimiter struct {
	rdb          *goredis.Client
	burst        float64
	refillPerSec float64
	keyPrefix    string
	script       *goredis.Script
}

// NewRedisLimiter wraps the given client. Keys are namespaced as
// "<keyPrefix>:<scope>:<key>"; pass an empty keyPrefix to use the default
// "ratelimit".
func NewRedisLimiter(rdb *goredis.Client, burst, refillPerSec float64, keyPrefix string) *RedisLimiter {
	if keyPrefix == "" {
		keyPrefix = "ratelimit"
	}
	return &RedisLimiter{
		rdb:          rdb,
		burst:        burst,
		refillPerSec: refillPerSec,
		keyPrefix:    keyPrefix,
		script:       goredis.NewScript(luaTokenBucket),
	}
}

// Allow consumes one token from the (key, scope) bucket. The Lua script
// guarantees atomicity across replicas.
//
// Returns (false, retryAfter, nil) when the bucket is empty;
// (false, _, err) on transport / scripting failures so the caller can
// decide whether to fail open.
func (l *RedisLimiter) Allow(ctx context.Context, key string, scope Scope) (bool, time.Duration, error) {
	if l.burst <= 0 || l.refillPerSec <= 0 {
		return false, time.Second, nil
	}

	bucketKey := fmt.Sprintf("%s:%s:%s", l.keyPrefix, scope, key)
	nowMs := time.Now().UnixMilli()

	// TTL: idle buckets refill to full in burst/refill seconds; double it
	// so we keep state long enough to avoid surprise resets at the edge.
	ttlSeconds := int64((l.burst/l.refillPerSec)*2) + 1
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	res, err := l.script.Run(ctx, l.rdb, []string{bucketKey},
		l.burst,
		l.refillPerSec,
		nowMs,
		ttlSeconds,
	).Result()
	if err != nil {
		return false, 0, fmt.Errorf("ratelimit redis: %w", err)
	}

	values, ok := res.([]any)
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("ratelimit redis: unexpected reply %T", res)
	}

	allowedRaw, _ := toInt64(values[0])
	retryAfterMsRaw, _ := toInt64(values[1])

	if allowedRaw == 1 {
		return true, 0, nil
	}
	retryAfter := time.Duration(retryAfterMsRaw) * time.Millisecond
	if retryAfter <= 0 {
		retryAfter = time.Millisecond
	}
	return false, retryAfter, nil
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// luaTokenBucket atomically refills then debits a token bucket. Inputs:
//
//	KEYS[1] — bucket hash key
//	ARGV[1] — burst (max tokens, float)
//	ARGV[2] — refillPerSec (tokens per second, float)
//	ARGV[3] — now in milliseconds (int)
//	ARGV[4] — TTL in seconds (int)
//
// Returns {allowed, retry_after_ms} where allowed is 0 or 1.
const luaTokenBucket = `
local key = KEYS[1]
local burst = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call('HMGET', key, 'tokens', 'last_ms')
local tokens = tonumber(data[1])
local last_ms = tonumber(data[2])

if tokens == nil or last_ms == nil then
    tokens = burst
    last_ms = now_ms
else
    local elapsed_ms = now_ms - last_ms
    if elapsed_ms > 0 then
        local refilled = (elapsed_ms / 1000.0) * refill
        tokens = math.min(burst, tokens + refilled)
        last_ms = now_ms
    end
end

local allowed = 0
local retry_after_ms = 0
if tokens >= 1 then
    tokens = tokens - 1
    allowed = 1
else
    local missing = 1 - tokens
    retry_after_ms = math.ceil((missing / refill) * 1000)
    if retry_after_ms < 1 then
        retry_after_ms = 1
    end
end

redis.call('HSET', key, 'tokens', tokens, 'last_ms', last_ms)
redis.call('EXPIRE', key, ttl)

return {allowed, retry_after_ms}
`
