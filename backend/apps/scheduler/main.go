package main

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
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
		log.Error("invalid config", err)
		os.Exit(1)
	}

	shutdownTracing := tracing.Init("jobradar-scheduler", cfg.Observability.JaegerEndpoint, log)

	database, err := db.OpenWithRetry(cfg.DB, 2, dbMaxAttempts, dbRetryDelay)
	if err != nil {
		log.Error("db open failed", err)
		shutdownTracing()
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.PoolReady(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer shutdownTracing()

	svc := scansvc.NewSchedulerService(scansvc.NewQueueStore(database))
	checkInterval := time.Duration(cfg.Scheduler.CheckIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Scheduler.StuckTimeoutMinutes) * time.Minute
	jitter := computeJitter(checkInterval, rand.New(rand.NewSource(time.Now().UnixNano())))

	log.Info("started",
		logger.Int(logger.FieldScanIntervalMinutes, cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur(logger.FieldCheckInterval, checkInterval),
		logger.Dur(logger.FieldStuckTimeout, stuckTimeout),
		logger.Dur(logger.FieldJitter, jitter),
	)

	Run(ctx, log, svc, cfg.Scheduler.ScanIntervalMinutes, checkInterval+jitter, stuckTimeout)
}
