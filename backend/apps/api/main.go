package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	shutdown := tracing.Init("jobradar-api", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB, 10)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	app, err := InitializeApp(database, cfg)
	if err != nil {
		log.Error("wire initialization failed", err)
		os.Exit(1)
	}

	router := NewRouter(log, database, app.Handlers())

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", logger.Str("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", err)
			os.Exit(1)
		}
	}()

	go runRefresher(ctx, log, dashboardsvc.NewDashboardRefresher(database))

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
}

// runRefresher keeps dashboard materialized views up to date.
// Pipeline stats refresh every 3 minutes; the heatmap every 5 minutes.
func runRefresher(ctx context.Context, log logger.Logger, r dashboardsvc.StatsRefresher) {
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
