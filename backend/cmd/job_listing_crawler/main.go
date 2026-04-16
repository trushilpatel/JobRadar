package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/pipeline/crawler"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
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

	companies := companysvc.NewStore(database)
	ledger := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queue := scansvc.NewQueueStore(database)
	recorder := scansvc.NewCrawlerService(scanLog, companies)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	googleCfg := google.Config{
		MinDelayMs:  cfg.GoogleScraperMinDelayMs,
		MaxDelayMs:  cfg.GoogleScraperMaxDelayMs,
		TimeoutMs:   cfg.GoogleScraperTimeoutMs,
		MaxPages:    cfg.GoogleScraperMaxPages,
		EnrichMinMs: cfg.GoogleScraperEnrichMinMs,
		EnrichMaxMs: cfg.GoogleScraperEnrichMaxMs,
	}
	engine := crawler.NewEngine(companies, ledger, recorder, producer, googleCfg, log)

	// Reset any rows stuck in processing from a previous crash.
	if err := queue.ResetStuck(stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
		// non-fatal: continue
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	// Poll once immediately on startup, then on every tick.
	pollAndProcess(ctx, log, queue, engine, companyDelay)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pollAndProcess(ctx, log, queue, engine, companyDelay)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func pollAndProcess(ctx context.Context, log logger.Logger, queue scansvc.ScanQueueStore, engine *crawler.Engine, companyDelay time.Duration) {
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

	if companyDelay > 0 {
		log.Debug("pacing before next claim", logger.Dur("delay", companyDelay))
		select {
		case <-time.After(companyDelay):
		case <-ctx.Done():
		}
	}
}
