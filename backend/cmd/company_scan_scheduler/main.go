package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
)

// checkInterval is how often the scheduler queries the DB for due companies.
// Intentionally short — the query is cheap and we want low latency on the trigger.
const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicCompanyScanRequest); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	companies := store.NewCompanyStore(database)

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
	)

	// Run once immediately on startup, then on every tick.
	enqueuedue(ctx, log, companies, producer, cfg.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, log, companies, producer, cfg.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

// enqueuedue queries companies that are due for a scan and publishes their IDs
// to TopicCompanyScanJob. The scan worker will pick them up and run RunForCompany.
func enqueuedue(ctx context.Context, log logger.Logger, companies store.CompanyReader, pub kafka.Publisher, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	for _, c := range due {
		payload, err := json.Marshal(scanRequest{CompanyID: c.ID})
		if err != nil {
			log.Error("marshal failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		if err := pub.Publish(ctx, kafka.TopicCompanyScanRequest, payload); err != nil {
			log.Error("publish failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
