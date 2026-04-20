package store

import (
	"context"
	"time"
)

// CooldownStatus captures a slot's current failure count and the instant
// after which the slot becomes available again.
type CooldownStatus struct {
	FailureCount  int
	CooldownUntil time.Time
}

// CooldownPolicy controls exponential backoff timing.
//
// Duration is computed as Base * 2^(failureCount-1), capped at Max, then
// perturbed by a uniform jitter in [-Jitter, +Jitter] fraction.
type CooldownPolicy struct {
	Base   time.Duration
	Max    time.Duration
	Jitter float64
}

// LLMCooldownStore tracks per-slot circuit-breaker state for the router.
type LLMCooldownStore interface {
	IsAvailable(ctx context.Context, slotID string) (bool, error)
	RecordSuccess(ctx context.Context, slotID string) error
	RecordFailure(ctx context.Context, slotID string) error
	Status(ctx context.Context, slotID string) (CooldownStatus, error)
}
