package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	shutdown := tracing.Init("jobradar-filter", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, consumer, producer, err := initInfra(ctx, cfg, log)
	if err != nil {
		os.Exit(1)
	}
	defer consumer.Close()
	defer producer.Close()

	worker, err := buildWorker(cfg, database, consumer, producer, log)
	if err != nil {
		os.Exit(1)
	}

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}

func initInfra(ctx context.Context, cfg *config.Config, log logger.Logger) (*gorm.DB, *kafka.Consumer, *kafka.Producer, error) {
	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		return nil, nil, nil, err
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(ctx, cfg.Kafka, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		return nil, nil, nil, err
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		return nil, nil, nil, err
	}

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		return nil, nil, nil, err
	}

	return database, consumer, producer, nil
}

func buildWorker(cfg *config.Config, database *gorm.DB, consumer *kafka.Consumer, producer *kafka.Producer, log logger.Logger) (*Worker, error) {
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
		return nil, err
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	return NewWorker(consumer, producer, scorer, ledger,
		cfg.Filter.BatchSize, cfg.Filter.FlushTimeoutSeconds, cfg.Filter.LLMParallelism, log), nil
}
