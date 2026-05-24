package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	commonredis "github.com/trushilpatel/jobradar/common/redis"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
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
	cfg := config.Load()
	log := logger.Init("api")

	// Surface unmapped errors that fall through to the 500 path. Without this,
	// handler.HandleError silently returns "internal server error" with no log
	// line — diagnosing a 500 in production then requires reading code.
	handler.SetLogger(log)

	if err := cfg.Auth.Validate(); err != nil {
		log.Error("auth config invalid", err)
		os.Exit(1)
	}

	shutdown := tracing.Init("jobradar-api", cfg.Observability.JaegerEndpoint, log)
	defer shutdown()

	database, err := db.Open(cfg.DB, 10)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	app, err := InitializeApp(database, cfg, log)
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
	var supabaseClient supabaseadmin.Client
	if cfg.MultiUser.SupabaseURL != "" && cfg.MultiUser.SupabaseServiceRoleKey != "" {
		c, err := supabaseadmin.NewRESTClient(cfg.MultiUser.SupabaseURL, cfg.MultiUser.SupabaseServiceRoleKey, nil)
		if err != nil {
			// Non-fatal: in dev / test we ship without Supabase admin creds
			// and /api/me DELETE fails loudly when invoked. GET + PATCH
			// still work without Supabase.
			log.Warn("supabase admin client unavailable", logger.Err(err))
		} else {
			supabaseClient = c
		}
	} else {
		log.Warn("supabase admin client disabled — SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY missing")
	}
	mu := buildMultiUser(database, cfg, muCache, muLimiter, supabaseClient, log, app.PipelineSvc(), app.InsightsHandler(), app.CompanyAdminHandler(), app.AdapterConfigHandler())

	apiGroups := append(app.Handlers(), mu.APIHandlers()...)
	apiRouter := NewRouter(log, cfg, authMW, mu.IdentityMW(), mu.RateLimitMW(), apiGroups, mu.AdminHandlers())
	opsRouter := NewOpsRouter(database)

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

	go runRefresher(ctx, log, dashboardsvc.NewDashboardRefresher(database))

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiSrv.Shutdown(shutdownCtx) //nolint:errcheck
	opsSrv.Shutdown(shutdownCtx) //nolint:errcheck
}

// buildMultiUserPlatform constructs the cache + rate-limiter pair used by
// the Phase 1 multi-user middleware. Falls back to in-memory implementations
// when Redis is unreachable — the goal is that a developer can boot the API
// without standing up Redis first.
//
// Rate-limit refill rate is read RPM ÷ 60. Burst equals refill * 2 so a quiet
// caller has a one-minute headroom to absorb a small spike before getting
// throttled.
func buildMultiUserPlatform(cfg *config.Config, log logger.Logger) (cache.Cache, ratelimit.Limiter) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb, err := commonredis.New(ctx, cfg.Redis.URL)
	if err != nil {
		log.Warn("redis unavailable; using in-memory cache + limiter", logger.Err(err))
		return cache.NewMemoryCache(), ratelimit.NewMemoryLimiter(20, 0.5)
	}

	// Wrap Redis in a bounded in-process LRU. The same instance is fanned
	// out to the identity middleware and every service-layer cache decorator,
	// so any Set/Delete performed on this Cache clears the local layer as a
	// side effect — the user service does not need to know it exists.
	c := cache.NewLocalCache(
		cache.NewRedisCache(rdb),
		time.Duration(cfg.MultiUser.IdentityCacheLocalTTLSeconds)*time.Second,
		cfg.MultiUser.IdentityCacheLocalMaxEntries,
	)
	// Use the write RPM as the conservative budget — read RPM is a higher
	// limit but the limiter middleware picks the scope by HTTP method so
	// either RPM is honoured for its scope.
	burst := float64(cfg.MultiUser.RateLimitWriteRPM)
	if burst <= 0 {
		burst = 20
	}
	refill := burst / 60.0
	limiter := ratelimit.NewRedisLimiter(rdb, burst, refill, "ratelimit")
	return c, limiter
}

// runRefresher keeps dashboard materialized views up to date.
// Pipeline stats refresh every 3 minutes; the heatmap every 5 minutes.
// Both views are also refreshed once immediately on startup so the first
// API response reflects the current DB state rather than stale MV data.
func runRefresher(ctx context.Context, log logger.Logger, r dashboardsvc.StatsRefresher) {
	if err := r.RefreshPipelineStats(ctx); err != nil {
		log.Error("refresh pipeline stats failed", err)
	}
	if err := r.RefreshHeatmap(ctx); err != nil {
		log.Error("refresh heatmap failed", err)
	}

	pipelineTicker := time.NewTicker(3 * time.Minute)
	heatmapTicker := time.NewTicker(5 * time.Minute)
	defer pipelineTicker.Stop()
	defer heatmapTicker.Stop()

	for {
		select {
		case <-pipelineTicker.C:
			if err := r.RefreshPipelineStats(ctx); err != nil {
				log.Error("refresh pipeline stats failed", err)
			}
		case <-heatmapTicker.C:
			if err := r.RefreshHeatmap(ctx); err != nil {
				log.Error("refresh heatmap failed", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
