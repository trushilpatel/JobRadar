package common

import (
	"testing"
	"time"
)

// TestBackoff_BoundaryValues verifies the exponential back-off curve at the
// boundaries that matter: the first failure, one doubling, the point where the
// cap begins to dominate, and a huge count that must never overflow the shift.
func TestBackoff_BoundaryValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    int
		want time.Duration
	}{
		{"n=0 → base", 0, 5 * time.Minute},
		{"n=1 → base*2", 1, 10 * time.Minute},
		{"n=2 → base*4", 2, 20 * time.Minute},
		{"n=6 → base*64 = 320m", 6, 320 * time.Minute},
		{"n=7 → base*128 = 640m > cap → cap", 7, 6 * time.Hour},
		{"n=8 → cap", 8, 6 * time.Hour},
		{"n=maxShift → cap", BackoffMaxShift, 6 * time.Hour},
		{"huge n → cap (no shift overflow)", 1_000_000, 6 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Backoff(tc.n)
			if got != tc.want {
				t.Errorf("Backoff(%d) = %v, want %v", tc.n, got, tc.want)
			}
		})
	}
}

// TestBackoff_NegativeClampedToBase guards the defensive lower clamp: a caller
// passing a negative consecutive-error count (should never happen, but the
// signature is int) must not produce a fractional/negative shift.
func TestBackoff_NegativeClampedToBase(t *testing.T) {
	t.Parallel()
	if got := Backoff(-5); got != BackoffBase {
		t.Errorf("Backoff(-5) = %v, want base %v", got, BackoffBase)
	}
}

// TestBackoff_MonotonicNonDecreasing asserts the curve never decreases as the
// error count climbs — a regression here would let a chronically-failing
// company be re-served sooner than a fresher failure.
func TestBackoff_MonotonicNonDecreasing(t *testing.T) {
	t.Parallel()
	prev := time.Duration(-1)
	for n := 0; n <= BackoffMaxShift+4; n++ {
		got := Backoff(n)
		if got < prev {
			t.Fatalf("Backoff(%d)=%v decreased below Backoff(%d)=%v", n, got, n-1, prev)
		}
		if got > BackoffCap {
			t.Fatalf("Backoff(%d)=%v exceeded cap %v", n, got, BackoffCap)
		}
		prev = got
	}
}
