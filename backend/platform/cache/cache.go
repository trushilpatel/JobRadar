// Package cache provides a small key/value cache abstraction shared by the
// service-layer cache decorators.
//
// Three implementations ship with the package:
//
//   - RedisCache  — production: shared state across API replicas.
//   - MemoryCache — tests + single-process fallback.
//   - NoopCache   — disable caching entirely without changing call sites.
//
// All implementations honour the same contract: Get reports a clean miss when
// the key is absent or expired, Set serialises any JSON-encodable value, and
// Delete removes one or more keys atomically (per implementation's transport).
//
// GetOrLoad is the canonical helper for cache-aside reads: it does a single
// Get, falls back to a singleflight-deduplicated loader, and writes the loaded
// value back with the requested TTL.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache is the contract every implementation honours. All methods are safe
// for concurrent use.
//
// Get unmarshals the cached value into dest. A clean miss returns (false, nil)
// — error is reserved for transport / decoding failures.
type Cache interface {
	Get(ctx context.Context, key string, dest any) (hit bool, err error)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// loaderGroup deduplicates concurrent loader calls in GetOrLoad. It is a
// package-level instance so callers do not need to manage one themselves.
//
// Keying is per-cache-key — concurrent GetOrLoad calls for the same key
// share a single loader invocation; different keys never block each other.
var loaderGroup singleflight.Group

// GetOrLoad returns the cached value for key when present, otherwise invokes
// loader exactly once across all concurrent callers for the same key (via
// singleflight) and writes the result back into the cache with the given TTL.
//
// Cache transport failures are not fatal: GetOrLoad logs the error to the
// loader path so a misbehaving cache never breaks the read path. The same
// logic applies to the write-back: a Set failure does not surface to the
// caller — they still get the loaded value.
func GetOrLoad[T any](
	ctx context.Context,
	c Cache,
	key string,
	ttl time.Duration,
	loader func(ctx context.Context) (T, error),
) (T, error) {
	var zero T

	// Fast path: cache hit. A transport error here is treated as a miss so
	// callers degrade to the source of truth rather than failing loudly.
	if c != nil {
		var cached T
		hit, err := c.Get(ctx, key, &cached)
		if err == nil && hit {
			return cached, nil
		}
	}

	// Slow path: deduplicate concurrent loads for the same key.
	v, err, _ := loaderGroup.Do(key, func() (any, error) {
		val, loadErr := loader(ctx)
		if loadErr != nil {
			return zero, loadErr
		}
		if c != nil {
			// Best-effort write-back; never block the caller on a cache
			// failure here.
			_ = c.Set(ctx, key, val, ttl)
		}
		return val, nil
	})
	if err != nil {
		return zero, fmt.Errorf("cache: load %q: %w", key, err)
	}

	typed, ok := v.(T)
	if !ok {
		// singleflight returned a different concrete type — should never
		// happen because the loader always produces T, but guard anyway.
		return zero, fmt.Errorf("cache: singleflight type mismatch for key %q", key)
	}
	return typed, nil
}

// marshal is shared by the Memory and Redis implementations so Set/Get
// behaviour stays identical across backends.
func marshal(val any) ([]byte, error) {
	b, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("cache: marshal: %w", err)
	}
	return b, nil
}

func unmarshal(b []byte, dest any) error {
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("cache: unmarshal: %w", err)
	}
	return nil
}
