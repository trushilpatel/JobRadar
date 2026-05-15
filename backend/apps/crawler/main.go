package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/apple"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/ashby"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/deel"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/gem"
	atsgoogle "github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/greenhouse"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/lever"
	atsmeta "github.com/trushilpatel/jobradar/apps/crawler/ats/meta"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/microsoft"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/netflix"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/rippling"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/uber"
	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	stopTracing := tracing.Init("jobradar-crawler", cfg.Observability.JaegerEndpoint, log)
	defer stopTracing()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, producer, err := initInfra(ctx, cfg, log)
	if err != nil {
		os.Exit(1)
	}
	defer producer.Close()

	engine, rawQueue := buildEngine(database, producer, cfg, log)

	stuckTimeout := time.Duration(cfg.Crawler.StuckTimeoutSeconds) * time.Second
	if err := rawQueue.ResetStuck(ctx, stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items on startup", err)
	}

	// Periodic stuck-reset — runs every 5 minutes in the background.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := rawQueue.ResetStuck(ctx, stuckTimeout); err != nil {
					log.Error("failed to reset stuck queue items", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wrap scansvc.CrawlerQueueService → workers.CrawlerQueueService.
	queue := &queueAdapter{svc: rawQueue}

	googleCfg := atsgoogle.Config{
		MinDelayMs: cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs: cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:  cfg.Crawler.Google.TimeoutMs,
		MaxPages:   cfg.Crawler.Google.MaxPages,
	}

	// All adapters are pure-HTTP — no shared Chromium process needed.
	adapterRegistry := workers.NewRegistry()

	// ATS adapters — one goroutine per adapter type.
	adapterRegistry.Register(greenhouse.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(lever.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(ashby.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(netflix.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(uber.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(microsoft.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(rippling.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(apple.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(gem.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(deel.NewCrawlerService(queue, engine, log))

	// HTTP-scrape adapters (regex / JSON-LD over plain HTTP).
	adapterRegistry.Register(atsgoogle.NewCrawlerService(queue, engine, googleCfg, log))
	adapterRegistry.Register(atsmeta.NewCrawlerService(queue, engine, log))

	log.Info("starting multi-lane crawler", logger.Int("adapters", 12))

	var wg sync.WaitGroup
	adapterRegistry.StartAll(ctx, &wg)
	wg.Wait()

	log.Info("crawler stopped")
}

func initInfra(ctx context.Context, cfg *config.Config, log logger.Logger) (*gorm.DB, *kafka.Producer, error) {
	database, err := db.Open(cfg.DB, 5)
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
	scanSettings := companysvc.NewScanSettingsRepositoryService(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queueStore := scansvc.NewQueueStore(database)

	ledger := jobsvc.NewLedgerService(ledgerStore)
	recorder := scansvc.NewCrawlerService(scanLog)
	queue := scansvc.NewCrawlerQueueService(queueStore)

	googleCfg := atsgoogle.Config{
		MinDelayMs: cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs: cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:  cfg.Crawler.Google.TimeoutMs,
		MaxPages:   cfg.Crawler.Google.MaxPages,
	}

	engine := NewEngine(scanSettings, ledger, recorder, producer, googleCfg, log)
	return engine, queue
}

// queueAdapter bridges scansvc.CrawlerQueueService → workers.CrawlerQueueService.
// The two interfaces differ in ClaimNext: workers returns *workers.ScanQueueItem
// whereas the service layer returns *dto.ScanQueueItem. This adapter converts.
type queueAdapter struct {
	svc scansvc.CrawlerQueueService
}

func (a *queueAdapter) ClaimNext(ctx context.Context, adapterType string) (*workers.ScanQueueItem, error) {
	item, err := a.svc.ClaimNext(ctx, adapterType)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	return &workers.ScanQueueItem{
		ID:          item.ID,
		CompanyID:   item.CompanyID,
		AdapterType: item.AdapterType,
		Source:      item.Source,
	}, nil
}

func (a *queueAdapter) Complete(ctx context.Context, id string) error {
	return a.svc.Complete(ctx, id)
}

func (a *queueAdapter) UpdateHeartbeat(ctx context.Context, id string) error {
	return a.svc.UpdateHeartbeat(ctx, id)
}
