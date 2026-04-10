package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/api"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	jobStore := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	discardStore := store.NewDiscardStore(database)
	snapshotStore := store.NewSnapshotStore(database)
	appStore := store.NewApplicationStore(database)
	companyStore := store.NewCompanyStore(database)
	scanLogStore := store.NewCompanyScanLogStore(database)
	insightsStore := store.NewInsightsStore(database)
	scanQueueStore := store.NewScanQueueStore(database)

	jobHandler := handler.NewJobHandler(jobStore, ledger, discardStore, snapshotStore)
	appHandler := handler.NewApplicationHandler(appStore, jobStore)
	companyHandler := handler.NewCompanyHandler(companyStore, scanLogStore, scanQueueStore, cfg.ScanIntervalMinutes)
	dashboardHandler := handler.NewDashboardHandler(jobStore, scanLogStore, insightsStore)
	insightsHandler := handler.NewInsightsHandler(insightsStore)

	router := api.NewRouter(log, jobHandler, appHandler, companyHandler, dashboardHandler, insightsHandler)

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

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
}
