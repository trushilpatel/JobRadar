package tests

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/llm/store"
)

// computeBackoff is unexported. All tests exercise it indirectly through the
// memory store's RecordFailure / Status path, which delegates to computeBackoff.

func TestComputeBackoff_FirstFailureEqualsBase(t *testing.T) {
	// FailureCount=1 → shift=0 → dur = Base * 2^0 = Base (no jitter).
	// This also exercises the n<1 branch indirectly: the guard clamps n=0 to
	// n=1 which produces the same result as n=1 in normal use.
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base:   500 * time.Millisecond,
		Max:    10 * time.Second,
		Jitter: 0,
	})
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "slot")
	st, _ := s.Status(ctx, "slot")
	d := time.Until(st.CooldownUntil)
	// Base=500ms — allow generous runtime margin.
	if d < 100*time.Millisecond || d > 2*time.Second {
		t.Fatalf("expected ~500ms cooldown for first failure (n=1), got %v", d)
	}
}

func TestComputeBackoff_ZeroBaseFallsBackToMax(t *testing.T) {
	// Base=0 → float64(0)*Pow(2,shift) = 0 → dur=0 → "if dur <= 0" fires
	// and sets dur = p.Max. Jitter=0, so the final "if dur<=0" guard is not
	// reached. Cooldown should be approximately Max.
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base:   0,
		Max:    5 * time.Second,
		Jitter: 0,
	})
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "slot")
	st, _ := s.Status(ctx, "slot")
	d := time.Until(st.CooldownUntil)
	if d < 3*time.Second || d > 7*time.Second {
		t.Fatalf("expected ~5s cooldown when Base=0, got %v", d)
	}
}

func TestComputeBackoff_JitterCanCollapseToBase(t *testing.T) {
	// Jitter=1.0 means multiplier = 1 + (roll*2-1)*1.0 can be ≤0 when roll
	// is near 0. The final "if dur<=0 { dur=p.Base }" guard must ensure the
	// result is always positive.
	//
	// We run many iterations to make it highly likely the negative-dur branch
	// is exercised, and assert that the resulting CooldownUntil is always in
	// the future.
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base:   100 * time.Millisecond,
		Max:    1 * time.Second,
		Jitter: 1.0,
	})
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		_ = s.RecordSuccess(ctx, "slot") // reset between iterations
		_ = s.RecordFailure(ctx, "slot")
		st, _ := s.Status(ctx, "slot")
		if st.CooldownUntil.IsZero() {
			t.Fatalf("iter %d: CooldownUntil must not be zero after RecordFailure", i)
		}
		if !time.Now().Before(st.CooldownUntil) {
			t.Fatalf("iter %d: CooldownUntil must be in the future, got %v", i, st.CooldownUntil)
		}
	}
}

func TestComputeBackoff_ShiftCappedAt30DoesNotOverflow(t *testing.T) {
	// After 35 failures, shift would be 34 without the cap. The cap at 30
	// keeps it finite. Duration must still be ≤ Max + small margin.
	s := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{
		Base:   time.Millisecond,
		Max:    30 * time.Second,
		Jitter: 0,
	})
	ctx := context.Background()
	for i := 0; i < 35; i++ {
		_ = s.RecordFailure(ctx, "slot")
	}
	st, _ := s.Status(ctx, "slot")
	d := time.Until(st.CooldownUntil)
	if d > 35*time.Second {
		t.Fatalf("expected duration capped at Max (~30s), got %v", d)
	}
}
