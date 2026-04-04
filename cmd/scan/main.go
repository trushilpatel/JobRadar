package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/scan"
)

const scanInterval = 30 * time.Minute

func main() {
	cfg := config.Load()

	// --- DB ---
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// --- Kafka producer ---
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka: %v", err)
	}
	defer producer.Close()

	// --- Store layer ---
	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewJobScanLogStore(database)

	// --- Engine ---
	engine := scan.NewEngine(companies, ledger, scanLog, producer)

	// --- Run loop ---
	// Trap SIGINT/SIGTERM so a clean shutdown drains the current scan
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Println("[scan] worker started")
	engine.RunOnce(ctx) // run immediately on startup

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			engine.RunOnce(ctx)
		case <-ctx.Done():
			log.Println("[scan] shutting down")
			return
		}
	}
}
