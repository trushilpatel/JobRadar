package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/store"
	"github.com/trushilpatel/jobradar/pipeline/crawler"
)

const pollInterval = 10 * time.Second
const stuckTimeout = 30 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewCompanyScanLogStore(database)
	queue := store.NewScanQueueStore(database)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	googleCfg := google.Config{
		MinDelayMs:  cfg.GoogleScraperMinDelayMs,
		MaxDelayMs:  cfg.GoogleScraperMaxDelayMs,
		TimeoutMs:   cfg.GoogleScraperTimeoutMs,
		MaxPages:    cfg.GoogleScraperMaxPages,
		EnrichMinMs: cfg.GoogleScraperEnrichMinMs,
		EnrichMaxMs: cfg.GoogleScraperEnrichMaxMs,
	}
	engine := crawler.NewEngine(companies, ledger, scanLog, producer, companyDelay, googleCfg, log)

	// Reset any rows stuck in processing from a previous crash.
	if err := queue.ResetStuck(stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
		// non-fatal: continue
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	// Poll once immediately on startup, then on every tick.
	pollAndProcess(ctx, log, queue, engine)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pollAndProcess(ctx, log, queue, engine)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func pollAndProcess(ctx context.Context, log logger.Logger, queue store.ScanQueueStore, engine *crawler.Engine) {
	item, err := queue.ClaimNext(ctx)
	if err != nil {
		log.Error("failed to claim queue item", err)
		return
	}
	if item == nil {
		log.Debug("queue empty")
		return
	}

	log.Info("processing company from queue",
		logger.Str(logger.FieldCompany, item.CompanyID),
		logger.Str("source", item.Source),
	)

	engine.RunForCompany(ctx, item.CompanyID)

	if err := queue.Complete(item.ID); err != nil {
		log.Error("failed to complete queue item", err, logger.Str(logger.FieldCompany, item.CompanyID))
	}
}
