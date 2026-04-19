package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/platform/db"
	"github.com/trushilpatel/jobradar/platform/kafka"
	"github.com/trushilpatel/jobradar/platform/logger"
	"github.com/trushilpatel/jobradar/platform/metrics"
	"github.com/trushilpatel/jobradar/platform/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	shutdown := tracing.Init("jobradar-filter", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(context.Background(), cfg.Kafka, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	scorer, err := llmcore.NewScorer(llmcore.Options{
		Provider:    cfg.LLM.Provider,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.Filter.Model,
		TargetRoles: cfg.LLM.TargetRoles,
		YearsExp:    cfg.LLM.YearsExp,
		Log:         log,
		Limits: llmcore.RateLimits{
			RPM:   cfg.Filter.RateLimits.RPM,
			RPD:   cfg.Filter.RateLimits.RPD,
			TPMIn: cfg.Filter.RateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	worker := NewWorker(consumer, producer, scorer, ledger,
		cfg.Filter.BatchSize, cfg.Filter.FlushTimeoutSeconds, cfg.Filter.LLMParallelism, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
