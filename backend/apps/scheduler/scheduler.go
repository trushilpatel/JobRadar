package main

import (
	"context"
	"math/rand"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
)

// computeJitter returns a duration uniformly distributed in
// `[0, checkInterval/10)` using the supplied random source.
//
// Jitter spreads scheduler ticks across replicas so they do not all hammer
// the database at the same second. Returns 0 when checkInterval/10 == 0 to
// avoid `rand.Int63n` panicking on a non-positive bound — that case only
// occurs in tests with sub-10ns intervals but the guard keeps the helper
// safe to call from any caller.
func computeJitter(checkInterval time.Duration, src *rand.Rand) time.Duration {
	bound := int64(checkInterval) / 10
	if bound <= 0 {
		return 0
	}
	if src == nil {
		return time.Duration(rand.Int63n(bound))
	}
	return time.Duration(src.Int63n(bound))
}

// Run executes one immediate scheduler cycle and then loops on a ticker
// driven by `tickInterval`, invoking runCycle on each tick. It returns
// when `ctx` is cancelled.
//
// All dependencies are injected so the loop is testable without spinning
// up the real Postgres-backed scheduler service.
func Run(
	ctx context.Context,
	log logger.Logger,
	svc SchedulerService,
	scanIntervalMinutes int,
	tickInterval time.Duration,
	stuckTimeout time.Duration,
) {
	// Run immediately on startup so the first tick doesn't wait a full interval.
	runCycle(ctx, log, svc, scanIntervalMinutes, stuckTimeout)

	if tickInterval <= 0 {
		// Defensive: a non-positive ticker bound would panic time.NewTicker.
		// Treat as "single cycle then exit" so callers never crash.
		return
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runCycle(ctx, log, svc, scanIntervalMinutes, stuckTimeout)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

// runCycle performs one full scheduler pass:
//  1. Resets any processing rows whose crawler has gone silent (stuck detection).
//  2. Removes queued rows for companies that have since been deactivated.
//  3. Enqueues all companies currently due for a scan in a single query.
func runCycle(ctx context.Context, log logger.Logger, svc SchedulerService, intervalMinutes int, stuckTimeout time.Duration) {
	start := time.Now()

	if resetCount, err := svc.ResetStuck(ctx, stuckTimeout); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("reset_stuck").Inc()
		log.Error("reset stuck failed", err)
	} else {
		metrics.SchedulerStuckChecksTotal.Inc()
		if resetCount > 0 {
			metrics.SchedulerStuckRowsResetTotal.Add(float64(resetCount))
			log.Info("reset stuck queue rows", logger.Int(logger.FieldCount, resetCount))
		}
	}

	if reclaimed, err := svc.ReclaimStuckFetchRows(ctx); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("reclaim_stuck_fetch").Inc()
		log.Error("reclaim stuck fetch failed", err)
	} else if reclaimed > 0 {
		log.Info("reclaimed stuck fetch ledger rows", logger.Int(logger.FieldCount, int(reclaimed)))
	}

	if purged, err := svc.PurgeInactive(ctx); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("purge_inactive").Inc()
		log.Error("purge inactive failed", err)
	} else if purged > 0 {
		log.Info("purged inactive company queue entries", logger.Int(logger.FieldPurged, purged))
	}

	if n, err := svc.AutoPauseChronicFailures(ctx); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("auto_pause").Inc()
		log.Error("auto-pause chronic failures failed", err)
	} else if n > 0 {
		log.Info("auto-paused chronic-failing companies", logger.Int(logger.FieldCount, n))
	}

	if rerr := svc.RetryDueJobs(ctx); rerr != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("retry_due_jobs").Inc()
		log.Error("retry due jobs failed", rerr)
	}

	if n, eerr := svc.EscalateStuckRetryJobs(ctx); eerr != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("escalate_stuck_retry").Inc()
		log.Error("escalate stuck retry jobs failed", eerr)
	} else if n > 0 {
		log.Info("escalated stuck retry jobs to error",
			logger.Int(logger.FieldCount, int(n)),
		)
	}

	res, err := svc.EnqueueDue(ctx, intervalMinutes)
	if err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("enqueue_due").Inc()
		log.Error("enqueue due failed", err)
	} else {
		metrics.SchedulerCompaniesEnqueuedTotal.Add(float64(res.Enqueued))
	}

	// Phase 2: surface companies skipped because they have no scan_settings
	// row yet. Logged at INFO once per tick so admins can see the count even
	// when no companies were enqueued (e.g. immediately after migration when
	// the queue is idle but every company still needs onboarding).
	if res.SkippedNoSettings > 0 {
		log.Info("companies skipped — no scan_settings configured",
			logger.Int(logger.FieldSkippedNoSettings, res.SkippedNoSettings),
		)
	}

	metrics.SchedulerRunDuration.Observe(time.Since(start).Seconds())
	if res.Enqueued > 0 {
		log.Info("cycle complete",
			logger.Int(logger.FieldEnqueued, res.Enqueued),
			logger.Dur(logger.FieldDuration, time.Since(start)),
		)
	} else {
		log.Debug("cycle complete", logger.Dur(logger.FieldDuration, time.Since(start)))
	}
}
