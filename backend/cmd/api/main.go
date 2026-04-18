package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/api"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/tracing"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	shutdown := tracing.Init("jobradar-api", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	app, err := InitializeApp(database, cfg)
	if err != nil {
		log.Error("wire initialization failed", err)
		os.Exit(1)
	}

	router := api.NewRouter(log, database, app.Handlers())

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
