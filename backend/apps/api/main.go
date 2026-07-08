package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/apispec"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	clerkadmin "github.com/trushilpatel/jobradar/platform/clerkadmin"
)

// Strict server timeouts applied to both the public API listener and the
// internal ops listener. Short header/read timeouts blunt slowloris-style
// attacks; a bounded write timeout caps per-request resource usage.
const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 15 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 60 * time.Second
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
	log := logger.Init("api")

	// Surface unmapped errors that fall through to the 500 path. Without this,
	// handler.HandleError silently returns "internal server error" with no log
	// line — diagnosing a 500 in production then requires reading code.
	handler.SetLogger(log)
	// Huma migration foundation (Phase 0) — mirrors handler.SetLogger above,
	// and installs the legacy error envelope for errors Huma itself raises
	// (e.g. malformed request bodies) so migrated slices need zero frontend
	// changes. See docs/superpowers/plans/2026-06-13-huma-migration-plan.md.
	apihuma.SetLogger(log)
	apihuma.InstallLegacyErrorModel()

	if err := cfg.Auth.Validate(); err != nil {
		log.Error("auth config invalid", err)
		os.Exit(1)
	}

	shutdown := tracing.Init("jobradar-api", cfg.Observability.OTelEndpoint, log)
	defer shutdown()

	database, err := db.Open(cfg.DB, 10)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	// Built exactly once and threaded through: every /api-zone slice's
	// RestRouter registers its Huma operations onto this same config so they
	// share one zone document — see apispec.APIZoneConfig.
	apiZoneCfg := apispec.APIZoneConfig()
	// Same one-per-process rule for the /admin zone — see apispec.AdminZoneConfig.
	adminZoneCfg := apispec.AdminZoneConfig()

	app, err := InitializeApp(database, cfg, log, apiZoneCfg, adminZoneCfg)
	if err != nil {
		log.Error("wire initialization failed", err)
		os.Exit(1)
	}

	authMW, err := middleware.Auth(cfg.Auth, cfg.API.DevToken, log)
	if err != nil {
		log.Error("auth middleware init failed", err)
		os.Exit(1)
	}

	// Phase 1 multi-user services + /api/me handler. Cache and limiter fall
	// back to in-memory implementations when Redis is unreachable so the API
	// still boots in test/dev environments without a live Redis.
	muCache, muLimiter := buildMultiUserPlatform(cfg, log)

	// Clerk admin client — required for /api/me DELETE and /admin/users DELETE.
	// Non-fatal: in dev / test environments CLERK_SECRET_KEY may be unset;
	// delete endpoints fail loudly when invoked. GET + PATCH still work without it.
	clerkClient, clerkErr := clerkadmin.NewRESTClient(
		"https://api.clerk.dev",
		cfg.MultiUser.ClerkSecretKey,
		nil,
	)
	if clerkErr != nil {
		log.Warn("clerk admin client unavailable — account deletion degraded", logger.Err(clerkErr))
	}

	mu := buildMultiUser(database, cfg, muCache, muLimiter, clerkClient, log, app.CompanyAdminHandler(), app.AtsHandler(), app.PipelineHandler(), apiZoneCfg, adminZoneCfg)

	apiGroups := append(app.Handlers(), mu.APIHandlers()...)
	apiRouter := NewRouter(log, cfg, authMW, mu.IdentityMW(), mu.RateLimitMW(), apiGroups, mu.AdminHandlers())

	// Combined OpenAPI spec served on the ops listener (never behind /api or
	// /admin auth). Built AFTER NewRouter above, since that's what invokes
	// every RouteGroup.Register — including migrated slices' huma.Register
	// calls that populate apiZoneCfg.OpenAPI.Paths and adminZoneCfg.OpenAPI.Paths.
	// See apps/api/apispec for the current zone list.
	apiSpec, err := apispec.Build(
		apihuma.ZoneSpec{Prefix: "/api", OpenAPI: apiZoneCfg.OpenAPI},
		apihuma.ZoneSpec{Prefix: "/admin", OpenAPI: adminZoneCfg.OpenAPI},
	)
	if err != nil {
		log.Error("huma spec build failed", err)
		os.Exit(1)
	}
	apispec.ApplyPublicURL(apiSpec, cfg.API.PublicURL)
	opsRouter := NewOpsRouter(database, apiSpec)

	apiSrv := &http.Server{
		Addr:              ":8080",
		Handler:           apiRouter,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
	opsSrv := &http.Server{
		Addr:              cfg.Observability.MetricsAddr,
		Handler:           opsRouter,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("api listening", logger.Str("addr", apiSrv.Addr))
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("api server error", err)
			os.Exit(1)
		}
	}()

	go func() {
		log.Info("ops listening", logger.Str("addr", opsSrv.Addr))
		if err := opsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("ops server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiSrv.Shutdown(shutdownCtx) //nolint:errcheck
	opsSrv.Shutdown(shutdownCtx) //nolint:errcheck
}
