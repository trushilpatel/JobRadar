package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/pipeline/crawler"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	shutdown := tracing.Init("jobradar-crawler", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg.Kafka, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	// --- Stores (wiring layer only) ---
	companyStore := companysvc.NewStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queueStore := scansvc.NewQueueStore(database)

	// --- Services (what workers depend on) ---
	companies := companysvc.NewCrawlerReader(companyStore)
	ledger := jobsvc.NewLedgerService(ledgerStore)
	recorder := scansvc.NewCrawlerService(scanLog)
	queue := scansvc.NewCrawlerQueueService(queueStore)

	pollInterval := time.Duration(cfg.Crawler.PollIntervalSeconds) * time.Second
	stuckTimeout := time.Duration(cfg.Crawler.StuckTimeoutSeconds) * time.Second
	heartbeatInterval := time.Duration(cfg.Crawler.HeartbeatIntervalSeconds) * time.Second
	companyDelay := time.Duration(cfg.Crawler.CompanyDelaySeconds) * time.Second

	googleCfg := google.Config{
		MinDelayMs:  cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs:  cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:   cfg.Crawler.Google.TimeoutMs,
		MaxPages:    cfg.Crawler.Google.MaxPages,
		EnrichMinMs: cfg.Crawler.Google.EnrichMinMs,
		EnrichMaxMs: cfg.Crawler.Google.EnrichMaxMs,
	}
	engine := crawler.NewEngine(companies, ledger, recorder, producer, googleCfg, log)

	// Reset any rows stuck in processing from a previous crash.
	if err := queue.ResetStuck(ctx, stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	var wg sync.WaitGroup
	var active int32 // 0=idle, 1=scan in progress

	startScan := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&active, 0)
			pollAndProcess(ctx, log, queue, engine, companyDelay, heartbeatInterval)
		}()
	}

	// Poll once immediately on startup.
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

func pollAndProcess(ctx context.Context, log logger.Logger, queue scansvc.CrawlerQueueService, engine *crawler.Engine, companyDelay, heartbeatInterval time.Duration) {
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

// startHeartbeat ticks every interval and updates heartbeat_at in the DB for the
// given queue row. Call the returned stop func (or defer it) to end the goroutine.
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
