package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

const (
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

	svc := scansvc.NewSchedulerService(scansvc.NewQueueStore(database))
	refresher := dashboardsvc.NewDashboardRefresher(database)
	checkInterval := time.Duration(cfg.Scheduler.CheckIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Scheduler.StuckTimeoutMinutes) * time.Minute

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
		logger.Dur("stuck_timeout", stuckTimeout),
	)

	state := &refreshState{}

	// Run immediately on startup so the first tick doesn't wait a full interval.
	runCycle(ctx, log, svc, refresher, state, cfg.Scheduler.ScanIntervalMinutes, stuckTimeout)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runCycle(ctx, log, svc, refresher, state, cfg.Scheduler.ScanIntervalMinutes, stuckTimeout)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

// refreshState tracks when each materialized view was last refreshed.
type refreshState struct {
	lastPipelineStats time.Time
	lastHeatmap       time.Time
}

// runCycle performs one full scheduler pass:
//  1. Resets any processing rows whose crawler has gone silent (stuck detection).
//  2. Enqueues all companies currently due for a scan in a single query.
//  3. Refreshes dashboard materialized views on their own intervals.
func runCycle(ctx context.Context, log logger.Logger, svc scansvc.SchedulerService, refresher dashboardsvc.StatsRefresher, state *refreshState, intervalMinutes int, stuckTimeout time.Duration) {
	start := time.Now()

	if err := svc.ResetStuck(ctx, stuckTimeout); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("reset_stuck").Inc()
		log.Error("reset stuck failed", err)
	} else {
		metrics.SchedulerStuckChecksTotal.Inc()
	}

	n, err := svc.EnqueueDue(ctx, intervalMinutes)
	if err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("enqueue_due").Inc()
		log.Error("enqueue due failed", err)
	} else {
		metrics.SchedulerCompaniesEnqueuedTotal.Add(float64(n))
	}

	now := time.Now()

	if now.Sub(state.lastPipelineStats) >= 3*time.Minute {
		if err := refresher.RefreshPipelineStats(ctx); err != nil {
			log.Error("refresh pipeline stats failed", err)
		} else {
			state.lastPipelineStats = now
		}
	}

	if now.Sub(state.lastHeatmap) >= 5*time.Minute {
		if err := refresher.RefreshHeatmap(ctx); err != nil {
			log.Error("refresh heatmap failed", err)
		} else {
			state.lastHeatmap = now
		}
	}

	metrics.SchedulerRunDuration.Observe(time.Since(start).Seconds())
	log.Info("cycle complete",
		logger.Int(logger.FieldEnqueued, n),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
}
