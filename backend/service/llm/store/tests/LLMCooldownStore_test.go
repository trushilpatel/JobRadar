package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func newCooldownMem() store.LLMCooldownStore {
	return store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 60 * time.Second, Max: 3600 * time.Second, Jitter: 0})
}

func TestCooldownMem_InitiallyAvailable(t *testing.T) {
	s := newCooldownMem()
	ok, err := s.IsAvailable(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("should be available initially")
	}
}

func TestCooldownMem_RecordFailureProgression(t *testing.T) {
	s := newCooldownMem()
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	ok, _ := s.IsAvailable(ctx, "s1")
	if ok {
		t.Fatal("should be in cooldown after first failure")
	}
	st, err := s.Status(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if st.FailureCount != 1 {
		t.Fatalf("want count=1, got %d", st.FailureCount)
	}
}

func TestCooldownMem_RecordSuccessResets(t *testing.T) {
	s := newCooldownMem()
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1")
	_ = s.RecordSuccess(ctx, "s1")
	st, _ := s.Status(ctx, "s1")
	if st.FailureCount != 0 {
		t.Fatalf("want reset, got count=%d", st.FailureCount)
	}
	ok, _ := s.IsAvailable(ctx, "s1")
	if !ok {
		t.Fatal("should be available after success")
	}
}

func TestCooldownMem_ExponentialBackoff(t *testing.T) {
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 100 * time.Millisecond, Max: 1 * time.Hour, Jitter: 0})
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1") // ~100ms
	st1, _ := s.Status(ctx, "s1")
	_ = s.RecordFailure(ctx, "s1") // ~200ms
	st2, _ := s.Status(ctx, "s1")
	if !st2.CooldownUntil.After(st1.CooldownUntil) {
		t.Fatal("expected backoff to grow")
	}
	if st2.FailureCount != 2 {
		t.Fatalf("want count=2, got %d", st2.FailureCount)
	}
}

func TestCooldownMem_ShiftOverflowClamped(t *testing.T) {
	// computeBackoff clamps shift at 30 to avoid float overflow. Trigger this
	// path by recording many failures; the cooldown duration must still cap at
	// policy.Max.
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: 30 * time.Second, Jitter: 0})
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		_ = s.RecordFailure(ctx, "s1")
	}
	st, _ := s.Status(ctx, "s1")
	if d := time.Until(st.CooldownUntil); d > 35*time.Second {
		t.Fatalf("cooldown should cap at ~30s even after 50 failures, got %v", d)
	}
	if st.FailureCount != 50 {
		t.Fatalf("count should track every failure, got %d", st.FailureCount)
	}
}

func TestCooldownMem_MaxCap(t *testing.T) {
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: 15 * time.Second, Jitter: 0})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = s.RecordFailure(ctx, "s1")
	}
	st, _ := s.Status(ctx, "s1")
	// Duration should be capped at 15s; until-now should not exceed ~15s materially.
	if time.Until(st.CooldownUntil) > 20*time.Second {
		t.Fatalf("expected cooldown capped, got %v", time.Until(st.CooldownUntil))
	}
}

func newCooldownRedis(t *testing.T) (store.LLMCooldownStore, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s := store.NewLLMCooldownRedisStore(rdb, store.CooldownPolicy{Base: 60 * time.Second, Max: 3600 * time.Second, Jitter: 0})
	return s, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func TestCooldownRedis_InitiallyAvailable(t *testing.T) {
	s, cleanup := newCooldownRedis(t)
	defer cleanup()
	ok, err := s.IsAvailable(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("should be available initially")
	}
}

func TestCooldownRedis_RecordFailureProgression(t *testing.T) {
	s, cleanup := newCooldownRedis(t)
	defer cleanup()
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	ok, _ := s.IsAvailable(ctx, "s1")
	if ok {
		t.Fatal("should be in cooldown after first failure")
	}
	st, err := s.Status(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if st.FailureCount != 1 {
		t.Fatalf("want count=1, got %d", st.FailureCount)
	}
}

func TestCooldownRedis_RecordSuccessResets(t *testing.T) {
	s, cleanup := newCooldownRedis(t)
	defer cleanup()
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1")
	_ = s.RecordSuccess(ctx, "s1")
	st, _ := s.Status(ctx, "s1")
	if st.FailureCount != 0 {
		t.Fatalf("want reset, got count=%d", st.FailureCount)
	}
	ok, _ := s.IsAvailable(ctx, "s1")
	if !ok {
		t.Fatal("should be available after success")
	}
}

func TestCooldownRedis_ExponentialBackoffGrows(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()
	s := store.NewLLMCooldownRedisStore(rdb, store.CooldownPolicy{
		Base: 100 * time.Millisecond, Max: time.Hour, Jitter: 0,
	})
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1")
	st1, _ := s.Status(ctx, "s1")
	_ = s.RecordFailure(ctx, "s1")
	st2, _ := s.Status(ctx, "s1")
	if !st2.CooldownUntil.After(st1.CooldownUntil) {
		t.Fatalf("expected second cooldown to extend past the first; got %v vs %v",
			st1.CooldownUntil, st2.CooldownUntil)
	}
	if st2.FailureCount != 2 {
		t.Fatalf("want count=2, got %d", st2.FailureCount)
	}
}

func TestCooldownRedis_StatusOnFreshSlot(t *testing.T) {
	s, cleanup := newCooldownRedis(t)
	defer cleanup()
	st, err := s.Status(context.Background(), "never-touched")
	if err != nil {
		t.Fatalf("status should not error for missing slot: %v", err)
	}
	if st.FailureCount != 0 || !st.CooldownUntil.IsZero() {
		t.Fatalf("expected zero-value status, got %+v", st)
	}
}

func TestCooldownRedis_RecordFailureWithJitter(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()
	s := store.NewLLMCooldownRedisStore(rdb, store.CooldownPolicy{
		Base: time.Second, Max: time.Minute, Jitter: 0.5,
	})
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, "s1")
	// With Jitter=0.5 and Base=1s, computed dur is in [0.5s, 1.5s] approx.
	d := time.Until(st.CooldownUntil)
	if d <= 0 || d > 2*time.Second {
		t.Fatalf("expected cooldown within 2s window, got %v", d)
	}
}

func TestCooldownMem_MultipleSlotsAreIsolated(t *testing.T) {
	s := newCooldownMem()
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "alpha")
	st1, _ := s.Status(ctx, "alpha")
	st2, _ := s.Status(ctx, "beta")
	if st1.FailureCount != 1 || st2.FailureCount != 0 {
		t.Fatalf("slots must be isolated; alpha=%+v beta=%+v", st1, st2)
	}
}

func TestCooldownMem_RecordFailureWithJitter(t *testing.T) {
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base: time.Second, Max: time.Minute, Jitter: 0.5,
	})
	ctx := context.Background()
	if err := s.RecordFailure(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, "s1")
	d := time.Until(st.CooldownUntil)
	if d <= 0 || d > 2*time.Second {
		t.Fatalf("expected jittered cooldown within reasonable window, got %v", d)
	}
}

func TestCooldownMem_IsAvailableAfterExpiry(t *testing.T) {
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base: 1 * time.Millisecond, Max: 1 * time.Millisecond, Jitter: 0,
	})
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1")
	// Sleep past the cooldown window.
	time.Sleep(5 * time.Millisecond)
	ok, err := s.IsAvailable(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("slot should be available again after cooldown elapses")
	}
}
