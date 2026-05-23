package main

import (
	"context"
	"time"

	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// SchedulerService is the narrow interface the scheduler worker depends on.
// It mirrors scansvc.SchedulerService so the app never couples directly to
// the service package's exported type.
type SchedulerService interface {
	EnqueueDue(ctx context.Context, intervalMinutes int) (scansvc.SchedulerCycleResult, error)
	ResetStuck(ctx context.Context, staleDur time.Duration) error
	PurgeInactive(ctx context.Context) (int, error)
}
