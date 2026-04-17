package main

import (
	"context"
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
	companysvc "github.com/trushilpatel/jobradar/service/company"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	shutdown := tracing.Init("jobradar-scheduler", cfg.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	companies := companysvc.NewStore(database)
	queue := scansvc.NewQueueStore(database)

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
	)

	enqueuedue(ctx, log, companies, queue, cfg.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, log, companies, queue, cfg.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func enqueuedue(ctx context.Context, log logger.Logger, companies companysvc.SchedulerService, queue scansvc.ScanQueueStore, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(ctx, intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	for _, c := range due {
		if err := queue.Enqueue(ctx, c.ID, model.QueueSourceScheduler); err != nil {
			log.Error("enqueue failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		metrics.SchedulerCompaniesEnqueuedTotal.Inc()
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
