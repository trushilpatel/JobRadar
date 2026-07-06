package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	apimiddleware "github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
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
	if len(os.Args) > 1 && os.Args[1] == printSpecFlag {
		if err := printSpec(); err != nil {
			fmt.Fprintf(os.Stderr, "print-spec: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

	publisher := js.NewPublisher(njs)

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

	// Built exactly once and threaded through: NodeRestRouter/AdminRestRouter
	// register their Huma operations onto these so they share one document
	// per zone — see wire_spec.go.
	nodeZoneCfg := nodeZoneConfig()
	adminZoneCfg := adminZoneConfig()

	app := wireApp(database, cfg, log, publisher, authErr, nodeZoneCfg, adminZoneCfg)

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

	r.Route("/node", app.nodeRouter.Register)
	r.Route("/admin/crawler-nodes", app.adminGate.Register)

	// Combined OpenAPI spec + docs UI, served unauthenticated on the main
	// router — this binary has no separate ops listener the way apps/api
	// does; health checks already live here unauthenticated, so this is
	// consistent with the existing convention for this binary specifically.
	apihuma.MountCombinedSpec(r, buildOrchestratorSpec(nodeZoneCfg, adminZoneCfg))

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	// Background: reset stuck assignments every 5 minutes.
	go runResetStuck(ctx, app.assignStore, resetStuckInterval, log)

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
