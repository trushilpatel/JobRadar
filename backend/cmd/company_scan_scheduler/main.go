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
)

// checkInterval is how often the scheduler queries the DB for due companies.
// Intentionally short — the query is cheap and we want low latency on the trigger.
const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()

	// --- DB ---
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// --- Kafka: ensure topic exists before producing ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicCompanyScanRequest); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}
	defer producer.Close()

	// --- Store ---
	companies := store.NewCompanyStore(database)

	log.Printf("[company_scan_scheduler] started — scan interval=%dm, check every %s",
		cfg.ScanIntervalMinutes, checkInterval)

	// Run once immediately on startup, then on every tick.
	enqueuedue(ctx, companies, producer, cfg.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, companies, producer, cfg.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Println("[company_scan_scheduler] shutting down")
			return
		}
	}
}

// enqueuedue queries companies that are due for a scan and publishes their IDs
// to TopicCompanyScanJob. The scan worker will pick them up and run RunForCompany.
func enqueuedue(ctx context.Context, companies store.CompanyReader, pub kafka.Publisher, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(intervalMinutes)
	if err != nil {
		log.Printf("[company_scan_scheduler] failed to query due companies: %v", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Printf("[company_scan_scheduler] %d companies due for scan", len(due))

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	for _, c := range due {
		payload, err := json.Marshal(scanRequest{CompanyID: c.ID})
		if err != nil {
			log.Printf("[company_scan_scheduler] marshal failed for %s: %v", c.ID, err)
			continue
		}
		if err := pub.Publish(ctx, kafka.TopicCompanyScanRequest, payload); err != nil {
			log.Printf("[company_scan_scheduler] publish failed for %s: %v", c.ID, err)
			continue
		}
		log.Printf("[company_scan_scheduler] queued %s (%s)", c.Name, c.ID)
	}
}
