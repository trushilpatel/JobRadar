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
	if err := kafka.EnsureTopics(context.Background(), cfg, kafka.TopicJobsRaw, kafka.TopicJobsRawDetail); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}

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
	scorer_, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	// --- Store ---
	ledger := store.NewSeenLedgerStore(database)

	// --- Worker ---
	worker := scorer.NewWorker(consumer, producer, scorer_, ledger,
		cfg.ScorerBatchSize, cfg.ScorerFlushTimeoutSeconds)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("[scorer] worker exited with error: %v", err)
	}

	log.Println("[scorer] shutting down")
}
