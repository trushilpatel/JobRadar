package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/scorer"
)

func main() {
	cfg := config.Load()

	// --- DB ---
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// --- Kafka ---
	consumer, err := kafka.NewConsumer(cfg, "scorer-group", kafka.TopicJobsRaw)
	if err != nil {
		log.Fatalf("kafka consumer: %v", err)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}
	defer producer.Close()

	// --- LLM ---
	scorer_, err := llm.NewScorer(cfg.LLMProvider, cfg.LLMAPIKey, cfg.TargetRoles, cfg.YearsExp)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	// --- Store ---
	ledger := store.NewSeenLedgerStore(database)

	// --- Worker ---
	worker := scorer.NewWorker(consumer, producer, scorer_, ledger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("[scorer] worker exited with error: %v", err)
	}

	log.Println("[scorer] shutting down")
}
