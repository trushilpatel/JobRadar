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
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	crawlernodesvc "github.com/trushilpatel/jobradar/service/crawlernode/services"
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

	// Build a lazy publisher that dials NATS on first use and re-dials after
	// failures. The sweeper is always enabled — a NATS outage causes the
	// publisher to return an error (increments SweeperNATSUnavailableTotal) but
	// does not prevent the scheduler from enqueuing or resetting stuck rows
	// (AC-2 / RC-2). Jobs stay at reason='retry' until NATS recovers.
	lazyPub := newLazyPublisher(func(pubCtx context.Context) (scansvc.SweeperPublisher, error) {
		njs, err := js.ConnectManaged(pubCtx, js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
		if err != nil {
			return nil, err
		}
		if streamErr := js.EnsureStreams(pubCtx, njs); streamErr != nil {
			return nil, streamErr
		}
		return js.NewPublisher(njs), nil
	}, log)

	// Audit writer for the auto-pause sweep: one admin_audit_log row per company
	// auto-paused after crossing the consecutive-error threshold. Audit writes
	// are non-fatal — a failure never blocks the pause.
	auditWriter := auditsvc.NewImplService(auditsvc.NewRepositoryService(database))

	sweeperOpts := scansvc.SchedulerServiceOptions{
		Ledger:         jobsvc.NewSeenLedgerStore(database),
		Publisher:      lazyPub,
		Auditor:        auditWriter,
		NodeReader:     crawlernodesvc.NewNodeRepositoryService(database),
		SettingsReader: companysvc.NewScanSettingsRepositoryService(database),
		MaxRetries:     cfg.Scheduler.SweeperMaxRetries,
		Log:            log,
	}

	svc := scansvc.NewSchedulerServiceWithSweeper(scansvc.NewQueueStore(database), sweeperOpts)
	checkInterval := time.Duration(cfg.Scheduler.CheckIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Scheduler.StuckTimeoutMinutes) * time.Minute
	jitter := computeJitter(checkInterval, rand.New(rand.NewSource(time.Now().UnixNano())))

	log.Info("started",
		logger.Int(logger.FieldScanIntervalMinutes, cfg.Scheduler.ScanIntervalMinutes),
		logger.Dur(logger.FieldCheckInterval, checkInterval),
		logger.Dur(logger.FieldStuckTimeout, stuckTimeout),
		logger.Dur(logger.FieldJitter, jitter),
		logger.Bool(logger.FieldSweeperEnabled, true),
	)

	Run(ctx, log, svc, cfg.Scheduler.ScanIntervalMinutes, checkInterval+jitter, stuckTimeout)
}
