package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/apple"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/ashby"
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
	companysvc "github.com/trushilpatel/jobradar/service/company"
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

	// One shared Chromium process for the still-rod-based Meta adapter.
	// Each scan opens a new tab; the browser stays alive for the pod lifetime.
	// Google has its own pure-HTTP path (see ats/google/http.go) and does not
	// touch this browser.
	sharedBrowser, closeSharedBrowser := launchSharedBrowser(cfg.Crawler.Google.ChromiumPath, log)
	defer closeSharedBrowser()
	engine.WithBrowser(sharedBrowser)

	adapterRegistry := workers.NewRegistry()

	// API adapters — one goroutine per adapter type.
	adapterRegistry.Register(greenhouse.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(lever.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(ashby.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(netflix.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(uber.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(microsoft.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(rippling.NewCrawlerService(queue, engine, log))
	adapterRegistry.Register(apple.NewCrawlerService(queue, engine, log))

	// Browser adapters — share one Chromium via tab-per-scan.
	adapterRegistry.Register(atsgoogle.NewCrawlerService(queue, engine, googleCfg, log))
	adapterRegistry.Register(atsmeta.NewCrawlerService(queue, engine, log))

	log.Info("starting multi-lane crawler", logger.Int("adapters", 10))

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
	companyStore := companysvc.NewStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	scanLog := scansvc.NewLogStore(database)
	queueStore := scansvc.NewQueueStore(database)

	companies := companysvc.NewCrawlerReader(companyStore)
	ledger := jobsvc.NewLedgerService(ledgerStore)
	recorder := scansvc.NewCrawlerService(scanLog)
	queue := scansvc.NewCrawlerQueueService(queueStore)

	googleCfg := atsgoogle.Config{
		MinDelayMs: cfg.Crawler.Google.MinDelayMs,
		MaxDelayMs: cfg.Crawler.Google.MaxDelayMs,
		TimeoutMs:  cfg.Crawler.Google.TimeoutMs,
		MaxPages:   cfg.Crawler.Google.MaxPages,
	}

	engine := NewEngine(companies, ledger, recorder, producer, googleCfg, log)
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

// launchSharedBrowser starts one Chromium process for the lifetime of the pod.
// All rod-based adapters share it via tab-per-scan; no second process is ever started.
// Returns a close func that must be deferred by the caller.
func launchSharedBrowser(chromiumPath string, log logger.Logger) (*rod.Browser, func()) {
	headless := os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
	l := launcher.New().Headless(headless).
		Set("no-sandbox", "").
		Set("disable-setuid-sandbox", "").
		Set("disable-dev-shm-usage", "")

	switch {
	case chromiumPath != "":
		l = l.Bin(chromiumPath)
	case os.Getenv("CHROMIUM_PATH") != "":
		l = l.Bin(os.Getenv("CHROMIUM_PATH"))
	default:
		if path, found := launcher.LookPath(); found {
			l = l.Bin(path)
		}
	}

	controlURL, err := l.Launch()
	if err != nil {
		log.Error("shared browser launch failed — rod adapters will launch own browsers", err)
		return nil, func() {}
	}
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		log.Error("shared browser connect failed — rod adapters will launch own browsers", err)
		return nil, func() {}
	}
	log.Info("shared browser started", logger.Bool("headless", headless))
	return browser, func() { _ = browser.Close() }
}
