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
	"github.com/trushilpatel/jobradar/internal/worker/scan"
)

const scanInterval = 24 * time.Hour

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

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobsRaw, kafka.TopicJobsRawDetail, kafka.TopicCompanyScanJob); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}

	// --- Kafka producer ---
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}
	defer producer.Close()

	// --- Kafka consumer for manual scan triggers ---
	consumer, err := kafka.NewConsumer(cfg, "scan-trigger", kafka.TopicCompanyScanJob)
	if err != nil {
		log.Fatalf("kafka consumer: %v", err)
	}
	defer consumer.Close()

	// --- Store layer ---
	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewJobScanLogStore(database)

	// --- Engine ---
	engine := scan.NewEngine(companies, ledger, scanLog, producer)

	// --- Goroutine: consume company.scan_job for on-demand scans ---
	go func() {
		type scanRequest struct {
			CompanyID string `json:"company_id"`
		}
		err := consumer.Poll(ctx, func(_ string, value []byte) error {
			var req scanRequest
			if err := json.Unmarshal(value, &req); err != nil {
				log.Printf("[scan] malformed scan_job message: %v", err)
				return nil // don't fail the consumer on bad messages
			}
			if req.CompanyID == "" {
				log.Printf("[scan] scan_job message missing company_id")
				return nil
			}
			engine.RunForCompany(ctx, req.CompanyID)
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("[scan] scan_job consumer error: %v", err)
		}
	}()

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
