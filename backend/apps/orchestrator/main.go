package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	apimiddleware "github.com/trushilpatel/jobradar/apps/api/middleware"
	orchmiddleware "github.com/trushilpatel/jobradar/apps/orchestrator/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	crawlernodesvc "github.com/trushilpatel/jobradar/service/crawlernode/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 15 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 60 * time.Second

	resetStuckInterval = 5 * time.Minute
	listenAddr         = ":8082"
)

func main() {
	cfg := config.Load()
	log := logger.Init("orchestrator")

	handler.SetLogger(log)

	if cfg.Crawler.CrawlerTokenSecret == "" {
		log.Error("CRAWLER_TOKEN_SECRET env var is required", nil)
		os.Exit(1)
	}

	shutdown := tracing.Init("jobradar-orchestrator", cfg.Observability.OTelEndpoint, log)
	defer shutdown()

	database, err := db.Open(cfg.DB, 25)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	nc, njs, err := js.Connect(js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
	if err != nil {
		log.Error("nats connect failed", err)
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := js.EnsureStreams(ctx, njs); err != nil {
		log.Error("nats ensure streams failed", err)
		os.Exit(1)
	}

	// --- Wire service dependencies ---
	publisher := js.NewPublisher(njs)
	nodeStore := crawlernodesvc.NewNodeRepositoryService(database)
	assignStore := crawlernodesvc.NewAssignmentRepositoryService(database)
	seenLedger := jobsvc.NewSeenLedgerStore(database)
	ledgerSvc := jobsvc.NewLedgerService(seenLedger)
	ledgerStore := crawlernodesvc.NewLedgerRepositoryService(ledgerSvc)
	logStore := scansvc.NewLogStore(database)
	scanLogStore := crawlernodesvc.NewScanLogRepositoryService(logStore)
	scanSettings := crawlernodesvc.NewScanSettingsReader(database)

	implSvc := crawlernodesvc.NewImplService(
		nodeStore, assignStore, ledgerStore, scanLogStore,
		publisher, scanSettings, cfg.Crawler.CrawlerTokenSecret,
	)
	nodeSvc := crawlernodesvc.NewValidationService(implSvc)

	// --- REST handlers ---
	nodeRestSvc := crawlernodesvc.NewNodeRestService(nodeSvc)
	adminRestSvc := crawlernodesvc.NewAdminRestService(nodeSvc)
	authMw := orchmiddleware.RequireNodeToken(nodeStore, cfg.Crawler.CrawlerTokenSecret)
	nodeRouter := crawlernodesvc.NewNodeRestRouter(nodeRestSvc, authMw)
	adminRouter := crawlernodesvc.NewAdminRestRouter(adminRestSvc)

	// --- HTTP server ---
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(apimiddleware.Logger(log))

	r.Route("/node", nodeRouter.Register)
	r.Route("/admin/crawler-nodes", adminRouter.Register)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	// Background: reset stuck assignments every 5 minutes.
	go func() {
		ticker := time.NewTicker(resetStuckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := assignStore.ResetStuckForNodes(ctx, crawlernodecommon.HeartbeatStaleDuration)
				if err != nil {
					log.Error("ResetStuckForNodes failed", err)
				} else if n > 0 {
					log.Info("reset stuck assignments", logger.Int("count", n))
				}
			}
		}
	}()

	log.Info("orchestrator listening", logger.Str("addr", listenAddr))

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		log.Info("orchestrator shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("http server error", err)
			os.Exit(1)
		}
	}
}
