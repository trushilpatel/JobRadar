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
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/cache"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
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

	// atsCacheTTL matches the TTL apps/api/wire_gen.go uses for the same
	// AtsService read-through cache — inter_company_delay_s changes rarely
	// enough that a shared 5-minute staleness window is acceptable here too.
	atsCacheTTL = 5 * time.Minute
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

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.PoolReady(database) })

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

	// AtsService chain (mirrors apps/api/wire_gen.go's repository->impl->cache
	// chain, minus the validation/REST layers this internal caller has no use
	// for) backs the AdapterDelayGate's inter_company_delay_s lookups.
	atsStore := atssvc.NewAtsStore(database)
	atsRepositoryService := atssvc.NewRepositoryService(atsStore)
	atsImplService := atssvc.NewImplService(atsRepositoryService)
	atsService := atssvc.NewCacheService(atsImplService, cache.NewMemoryCache(), atsCacheTTL)

	delayGate := crawlernodesvc.NewAdapterDelayGate(atsService)
	assignStore := crawlernodesvc.NewAssignmentRepositoryService(database, delayGate)
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

	// Admin JWT pipeline init — populates the package-level JWT verifier
	// apimiddleware.VerifyJWT (called internally by RequireIdentity) reads
	// from. Deliberately NOT fatal on failure (unlike apps/api's identical
	// call): /node/* (the core crawl-assignment pipeline) never depends on
	// Clerk and must stay up even if Clerk is unreachable — wireAdminGate
	// mounts a 503 stub for /admin/crawler-nodes/* instead of crashing the
	// whole process. See wire_admin_gate.go.
	_, authErr := apimiddleware.Auth(cfg.Auth, cfg.API.DevToken, log)
	if authErr != nil {
		log.Error("orchestrator: auth init failed — /admin/crawler-nodes unavailable", authErr)
	}
	adminGate := wireAdminGate(database, cfg, log, authErr, adminRouter)

	// --- HTTP server ---
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(apimiddleware.Logger(log))
	r.Use(apimiddleware.Metrics())

	// Health endpoints — liveness (/healthz, cheap) and readiness (/readyz,
	// NATS-gated). Extracted into registerHealthRoutes so route registration
	// is always co-located with the handler definitions (see health.go).
	registerHealthRoutes(r, nc)

	r.Route("/node", nodeRouter.Register)
	r.Route("/admin/crawler-nodes", adminGate.Register)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	// Background: reset stuck assignments every 5 minutes.
	go runResetStuck(ctx, assignStore, resetStuckInterval, log)

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
