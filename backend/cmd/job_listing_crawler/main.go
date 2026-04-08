package main

import (
	"context"
	"encoding/json"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_crawler"
)

func main() {
	cfg := config.Load()

	// --- DB ---
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// --- Kafka: ensure topics exist before producing ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobListingsRaw); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}

	// --- Kafka producer (publishes to job_listings.raw) ---
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}
	defer producer.Close()

	// --- Kafka consumer (receives company IDs from scheduler + API trigger) ---
	consumer, err := kafka.NewConsumer(cfg, "job-listing-crawler", kafka.TopicCompanyScanRequest)
	if err != nil {
		log.Fatalf("kafka consumer: %v", err)
	}
	defer consumer.Close()

	// --- Store layer ---
	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewCompanyScanLogStore(database)

	// --- Engine ---
	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	engine := job_listing_crawler.NewEngine(companies, ledger, scanLog, producer, companyDelay)

	log.Println("[scan] worker started — waiting for scan jobs on", kafka.TopicCompanyScanRequest)

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	err = consumer.Poll(ctx, func(_ string, value []byte) error {
		var req scanRequest
		if err := json.Unmarshal(value, &req); err != nil {
			log.Printf("[scan] malformed message: %v — skipping", err)
			return nil
		}
		if req.CompanyID == "" {
			log.Printf("[scan] message missing company_id — skipping")
			return nil
		}
		engine.RunForCompany(ctx, req.CompanyID)
		return nil
	})

	if err != nil && ctx.Err() == nil {
		log.Fatalf("[scan] consumer error: %v", err)
	}
	log.Println("[scan] shutting down")
}
