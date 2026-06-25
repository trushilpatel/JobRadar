// Package worker provides shared primitives used by NATS pipeline workers
// (filter, analyzer). Centralising these avoids drift across workers that
// must remain in sync with the same NATS consumer configuration.
package worker

import (
	"context"
	"time"
)

// Clock abstracts wall-clock reads and ctx-aware sleeps so tests can inject
// deterministic implementations without touching production code paths.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

// RealClock is the production Clock implementation.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
