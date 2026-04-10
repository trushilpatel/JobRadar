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
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_crawler"
)

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	consumer, err := kafka.NewConsumer(cfg, log, "job-listing-crawler", kafka.TopicCompanyScanRequest)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewCompanyScanLogStore(database)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	engine := job_listing_crawler.NewEngine(companies, ledger, scanLog, producer, companyDelay, log)

	log.Info("worker started", logger.Str("topic", kafka.TopicCompanyScanRequest))

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	err = consumer.Poll(ctx, func(_ string, value []byte) error {
		var req scanRequest
		if err := json.Unmarshal(value, &req); err != nil {
			log.Warn("malformed message, skipping", logger.Err(err))
			return nil
		}
		if req.CompanyID == "" {
			log.Warn("message missing company_id, skipping")
			return nil
		}
		engine.RunForCompany(ctx, req.CompanyID)
		return nil
	})

	if err != nil && ctx.Err() == nil {
		log.Error("consumer error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
