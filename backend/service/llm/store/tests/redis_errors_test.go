package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMiniRedis creates a miniredis instance plus a go-redis client and returns
// a cleanup function the caller must defer.
func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *goredis.Client, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return mr, rdb, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func defaultCooldownPolicy() store.CooldownPolicy {
	return store.CooldownPolicy{
		Base:   60 * time.Second,
		Max:    3600 * time.Second,
		Jitter: 0,
	}
}

// ---------------------------------------------------------------------------
// LLMCooldownRedisStore — error paths
// ---------------------------------------------------------------------------

// TestCooldownRedis_IsAvailable_RedisError forces an unexpected Redis error
// during the GET call and asserts IsAvailable returns (false, non-nil error).
func TestCooldownRedis_IsAvailable_RedisError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMCooldownRedisStore(rdb, defaultCooldownPolicy())

	// Record a failure first so the key exists, ensuring the code reaches the
	// Get call rather than returning early on Nil.
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "slot1"); err != nil {
		t.Fatalf("RecordFailure setup: %v", err)
	}

	// Now break Redis for all subsequent commands.
	mr.SetError("ERR forced error")
	defer mr.SetError("") // restore

	ok, err := s.IsAvailable(ctx, "slot1")
	if err == nil {
		t.Fatal("IsAvailable: expected error when Redis is broken, got nil")
	}
	if ok {
		t.Fatal("IsAvailable: expected false on Redis error, got true")
	}
}

// TestCooldownRedis_RecordSuccess_PipeError forces a pipeline error during
// RecordSuccess and confirms the error is returned.
func TestCooldownRedis_RecordSuccess_PipeError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMCooldownRedisStore(rdb, defaultCooldownPolicy())

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	if err := s.RecordSuccess(context.Background(), "slot1"); err == nil {
		t.Fatal("RecordSuccess: expected error when Redis is broken, got nil")
	}
}

// TestCooldownRedis_RecordFailure_ScriptError forces the Lua script to fail
// and asserts RecordFailure returns the wrapped error.
func TestCooldownRedis_RecordFailure_ScriptError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMCooldownRedisStore(rdb, defaultCooldownPolicy())

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	if err := s.RecordFailure(context.Background(), "slot1"); err == nil {
		t.Fatal("RecordFailure: expected error when Redis is broken, got nil")
	}
}

// TestCooldownRedis_Status_PipeError forces the pipeline to fail during Status
// and asserts a wrapped error is returned.
func TestCooldownRedis_Status_PipeError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMCooldownRedisStore(rdb, defaultCooldownPolicy())

	// Seed at least one key so the pipeline can't short-circuit on Nil.
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "slot1"); err != nil {
		t.Fatalf("setup RecordFailure: %v", err)
	}

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	if _, err := s.Status(ctx, "slot1"); err == nil {
		t.Fatal("Status: expected error when Redis is broken, got nil")
	}
}

// ---------------------------------------------------------------------------
// LLMRateLimitRedisStore — error paths
// ---------------------------------------------------------------------------

// TestRateLimitRedis_Reserve_ScriptError forces the Lua reserve script to fail
// and asserts Reserve returns a wrapped error.
func TestRateLimitRedis_Reserve_ScriptError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMRateLimitRedisStore(rdb)

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	_, err := s.Reserve(context.Background(), store.ReserveReq{
		SlotID: "slot1", ReqID: "req1", RPM: 10, RPD: 100,
	})
	if err == nil {
		t.Fatal("Reserve: expected error when Redis is broken, got nil")
	}
}

// TestRateLimitRedis_Release_ScriptError forces the release Lua script to
// return a real (non-Nil) error and asserts it is surfaced.
func TestRateLimitRedis_Release_ScriptError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMRateLimitRedisStore(rdb)

	// Reserve first so the release script has something to work with.
	ctx := context.Background()
	if _, err := s.Reserve(ctx, store.ReserveReq{
		SlotID: "slot1", ReqID: "req1", RPM: 10, RPD: 100,
	}); err != nil {
		t.Fatalf("Reserve setup: %v", err)
	}

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	if err := s.Release(ctx, "slot1", "req1"); err == nil {
		t.Fatal("Release: expected error when Redis is broken, got nil")
	}
}

// TestRateLimitRedis_Usage_PipeError forces the pipeline to fail during Usage
// and asserts a wrapped error is returned.
func TestRateLimitRedis_Usage_PipeError(t *testing.T) {
	mr, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	s := store.NewLLMRateLimitRedisStore(rdb)

	// Reserve so the slot has data in Redis.
	ctx := context.Background()
	if _, err := s.Reserve(ctx, store.ReserveReq{
		SlotID: "slot1", ReqID: "req1", RPM: 10, RPD: 100, TPMIn: 500, Tokens: 50,
	}); err != nil {
		t.Fatalf("Reserve setup: %v", err)
	}

	mr.SetError("ERR forced error")
	defer mr.SetError("")

	if _, err := s.Usage(ctx, "slot1"); err == nil {
		t.Fatal("Usage: expected error when Redis is broken, got nil")
	}
}

// ---------------------------------------------------------------------------
// factory.NewRedisStores
// ---------------------------------------------------------------------------

// TestNewRedisStores_ReturnsCorrectTypes verifies that NewRedisStores returns
// non-nil stores of the expected interface types without panicking.
func TestNewRedisStores_ReturnsCorrectTypes(t *testing.T) {
	_, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	cfg := config.CooldownConfig{
		BaseSeconds: 30,
		MaxSeconds:  3600,
		Jitter:      0.1,
	}

	rateStore, cooldownStore := store.NewRedisStores(rdb, cfg)

	if rateStore == nil {
		t.Fatal("NewRedisStores: rateStore must not be nil")
	}
	if cooldownStore == nil {
		t.Fatal("NewRedisStores: cooldownStore must not be nil")
	}

	// Verify the returned stores are functional — a basic round-trip confirms
	// construction is correct, not just non-nil.
	ctx := context.Background()
	result, err := rateStore.Reserve(ctx, store.ReserveReq{
		SlotID: "factory-test", ReqID: "r1", RPM: 10, RPD: 100,
	})
	if err != nil {
		t.Fatalf("rateStore.Reserve: %v", err)
	}
	if result != store.ReserveOK {
		t.Fatalf("rateStore.Reserve: expected ReserveOK, got %v", result)
	}

	if err := cooldownStore.RecordFailure(ctx, "factory-test"); err != nil {
		t.Fatalf("cooldownStore.RecordFailure: %v", err)
	}
	ok, err := cooldownStore.IsAvailable(ctx, "factory-test")
	if err != nil {
		t.Fatalf("cooldownStore.IsAvailable: %v", err)
	}
	if ok {
		t.Fatal("cooldownStore.IsAvailable: expected false immediately after RecordFailure with Base=30s")
	}
}

// TestNewRedisStores_ZeroConfigIsValid confirms NewRedisStores does not panic
// when CooldownConfig fields are zero-valued.
func TestNewRedisStores_ZeroConfigIsValid(t *testing.T) {
	_, rdb, cleanup := newMiniRedis(t)
	defer cleanup()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewRedisStores with zero config panicked: %v", r)
		}
	}()

	rateStore, cooldownStore := store.NewRedisStores(rdb, config.CooldownConfig{})
	if rateStore == nil || cooldownStore == nil {
		t.Fatal("NewRedisStores with zero config returned nil store")
	}
}
