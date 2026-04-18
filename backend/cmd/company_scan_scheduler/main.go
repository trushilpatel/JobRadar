package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

const (
	checkInterval = 1 * time.Minute
	dbMaxAttempts = 10
	dbRetryDelay  = 3 * time.Second
)

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	if err := cfg.Scheduler.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "scheduler: invalid config: %v\n", err)
		os.Exit(1)
	}

	// Init tracing before any os.Exit so spans can be flushed on error paths.
	shutdownTracing := tracing.Init("jobradar-scheduler", cfg.Observability.JaegerEndpoint)

	database, err := db.OpenWithRetry(cfg.DB, dbMaxAttempts, dbRetryDelay)
	if err != nil {
		log.Error("db open failed", err)
		shutdownTracing()
		os.Exit(1)
	}

	// Scheduler is never concurrent — one connection is sufficient.
	if sqlDB, err := database.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer shutdownTracing()

	queue := scansvc.NewQueueStore(database)
	stuckTimeout := time.Duration(cfg.Scheduler.StuckTimeoutMinutes) * time.Minute

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
		logger.Dur("stuck_timeout", stuckTimeout),
	)

	// Run immediately on startup so the first tick doesn't wait a full minute.
	runCycle(ctx, log, queue, cfg.Scheduler.ScanIntervalMinutes, stuckTimeout)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runCycle(ctx, log, queue, cfg.Scheduler.ScanIntervalMinutes, stuckTimeout)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

// runCycle performs one full scheduler pass:
//  1. Resets any processing rows whose crawler has gone silent (stuck detection).
//  2. Enqueues all companies that are currently due for a scan.
func runCycle(ctx context.Context, log logger.Logger, queue scansvc.ScanQueueStore, intervalMinutes int, stuckTimeout time.Duration) {
	start := time.Now()

	resetStuck(ctx, log, queue, stuckTimeout)
	enqueued := enqueuedue(ctx, log, queue, intervalMinutes)

	metrics.SchedulerRunDuration.Observe(time.Since(start).Seconds())
	log.Info("cycle complete",
		logger.Int("enqueued", enqueued),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
}

// resetStuck promotes processing rows whose heartbeat (or started_at) is older
// than stuckTimeout back to queued so the crawler can pick them up again.
func resetStuck(ctx context.Context, log logger.Logger, queue scansvc.ScanQueueStore, stuckTimeout time.Duration) {
	if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("reset_stuck").Inc()
		log.Error("reset stuck failed", err)
		return
	}
	// ResetStuck doesn't return a count; we record one increment per successful call
	// so the metric is at minimum a liveness signal that the path is exercised.
	metrics.SchedulerStuckChecksTotal.Inc()
}

// enqueuedue atomically enqueues all companies due for a scan, one at a time,
// until no more are due. Returns the number of companies enqueued this pass.
func enqueuedue(ctx context.Context, log logger.Logger, queue scansvc.ScanQueueStore, intervalMinutes int) int {
	var count int
	for {
		// Respect shutdown: stop mid-enumeration if a signal arrived.
		if ctx.Err() != nil {
			return count
		}

		companyID, err := queue.EnqueueDue(ctx, intervalMinutes, model.QueueSourceScheduler)
		if err != nil {
			metrics.SchedulerErrorsTotal.WithLabelValues("enqueue_due").Inc()
			log.Error("enqueue due failed", err)
			return count
		}
		if companyID == "" {
			return count
		}

		metrics.SchedulerCompaniesEnqueuedTotal.Inc()
		log.Debug("queued company", logger.Str(logger.FieldCompany, companyID))
		count++
	}
}
