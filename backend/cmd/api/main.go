package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/api"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/store"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// --- Kafka: ensure topic exists, then create producer ---
	kafkaCtx := context.Background()
	if err := kafka.EnsureTopics(kafkaCtx, cfg, kafka.TopicCompanyScanJob); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}
	defer producer.Close()

	// --- Stores ---
	jobStore := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	discardStore := store.NewDiscardStore(database)
	appStore := store.NewApplicationStore(database)
	companyStore := store.NewCompanyStore(database)
	scanLogStore := store.NewJobScanLogStore(database)

	// --- Handlers ---
	jobHandler := handler.NewJobHandler(jobStore, ledger, discardStore)
	appHandler := handler.NewApplicationHandler(appStore, jobStore)
	companyHandler := handler.NewCompanyHandler(companyStore, scanLogStore, producer)

	// --- Router ---
	router := api.NewRouter(jobHandler, appHandler, companyHandler)

	// --- Server ---
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[api] listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[api] server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[api] shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
