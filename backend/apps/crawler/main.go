package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	stopTracing := tracing.Init("jobradar-crawler", cfg.Observability.JaegerEndpoint)
	defer stopTracing()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, producer, err := initInfra(ctx, cfg, log)
	if err != nil {
		os.Exit(1)
	}
	defer producer.Close()

	engine, queue := buildEngine(database, producer, cfg, log)

	runLoop(ctx, cfg, log, queue, engine)
}

func initInfra(ctx context.Context, cfg *config.Config, log logger.Logger) (*gorm.DB, *kafka.Producer, error) {
	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		return nil, nil, err
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(ctx, cfg.Kafka, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		return nil, nil, err
	}

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		return nil, nil, err
	}

	return database, producer, nil
}

func buildEngine(database *gorm.DB, producer *kafka.Producer, cfg *config.Config, log logger.Logger) (*Engine, scansvc.CrawlerQueueService) {
	companyStore := companysvc.NewStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queueStore := scansvc.NewQueueStore(database)

	companies := companysvc.NewCrawlerReader(companyStore)
	ledger := jobsvc.NewLedgerService(ledgerStore)
	recorder := scansvc.NewCrawlerService(scanLog)
	queue := scansvc.NewCrawlerQueueService(queueStore)

	googleCfg := google.Config{
		MinDelayMs:  cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs:  cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:   cfg.Crawler.Google.TimeoutMs,
		MaxPages:    cfg.Crawler.Google.MaxPages,
		EnrichMinMs: cfg.Crawler.Google.EnrichMinMs,
		EnrichMaxMs: cfg.Crawler.Google.EnrichMaxMs,
	}

	engine := NewEngine(companies, ledger, recorder, producer, googleCfg, log)
	return engine, queue
}

func runLoop(ctx context.Context, cfg *config.Config, log logger.Logger, queue scansvc.CrawlerQueueService, engine *Engine) {
	pollInterval := time.Duration(cfg.Crawler.PollIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Crawler.StuckTimeoutSeconds) * time.Second
	heartbeatInterval := time.Duration(cfg.Crawler.HeartbeatIntervalSeconds) * time.Second
	companyDelay := time.Duration(cfg.Crawler.CompanyDelaySeconds) * time.Second

	if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	var wg sync.WaitGroup
	var active int32

	startScan := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&active, 0)
			pollAndProcess(ctx, log, queue, engine, companyDelay, heartbeatInterval)
		}()
	}

	atomic.StoreInt32(&active, 1)
	startScan()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
				log.Error("failed to reset stuck queue items", err)
			}
			if !atomic.CompareAndSwapInt32(&active, 0, 1) {
				log.Debug("scan in progress, skipping tick")
				continue
			}
			startScan()
		case <-ctx.Done():
			log.Info("shutting down")
			wg.Wait()
			return
		}
	}
}

func pollAndProcess(ctx context.Context, log logger.Logger, queue scansvc.CrawlerQueueService, engine *Engine, companyDelay, heartbeatInterval time.Duration) {
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

	stopHeartbeat := startHeartbeat(ctx, queue, item.ID, heartbeatInterval, log)
	defer stopHeartbeat()

	engine.RunForCompany(ctx, item.CompanyID)

	if err := queue.Complete(ctx, item.ID); err != nil {
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

func startHeartbeat(ctx context.Context, q scansvc.CrawlerQueueService, id string, interval time.Duration, log logger.Logger) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := q.UpdateHeartbeat(hbCtx, id); err != nil {
					log.Warn("heartbeat update failed", logger.Str(logger.FieldQueueID, id), logger.Err(err))
				}
			case <-hbCtx.Done():
				return
			}
		}
	}()
	return cancel
}
