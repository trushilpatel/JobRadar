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
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
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

	shutdownTracing := tracing.Init("jobradar-scheduler", cfg.Observability.OTelEndpoint, log)

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

	// Connect to NATS and wire the sweeper publisher.
	// If NATS is unavailable the sweeper is disabled (no-op) so the scheduler
	// can still enqueue/reset stuck rows — a NATS outage must not stop scans.
	var sweeperOpts scansvc.SchedulerServiceOptions
	njs, natsErr := js.ConnectManaged(ctx, js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
	if natsErr != nil {
		log.Error("nats connect failed — sweeper disabled, scan scheduling continues", natsErr)
	} else {
		if streamErr := js.EnsureStreams(ctx, njs); streamErr != nil {
			log.Error("nats ensure streams failed — sweeper disabled", streamErr)
		} else {
			sweeperOpts = scansvc.SchedulerServiceOptions{
				Ledger:     jobsvc.NewSeenLedgerStore(database),
				Publisher:  js.NewPublisher(njs),
				MaxRetries: cfg.Scheduler.SweeperMaxRetries,
				Log:        log,
			}
		}
	}

	svc := scansvc.NewSchedulerServiceWithSweeper(scansvc.NewQueueStore(database), sweeperOpts)
	checkInterval := time.Duration(cfg.Scheduler.CheckIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Scheduler.StuckTimeoutMinutes) * time.Minute
	jitter := computeJitter(checkInterval, rand.New(rand.NewSource(time.Now().UnixNano())))

	sweeperEnabled := sweeperOpts.Publisher != nil
	log.Info("started",
		logger.Int(logger.FieldScanIntervalMinutes, cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur(logger.FieldCheckInterval, checkInterval),
		logger.Dur(logger.FieldStuckTimeout, stuckTimeout),
		logger.Dur(logger.FieldJitter, jitter),
		logger.Bool(logger.FieldSweeperEnabled, sweeperEnabled),
	)

	Run(ctx, log, svc, cfg.Scheduler.ScanIntervalMinutes, checkInterval+jitter, stuckTimeout)
}
