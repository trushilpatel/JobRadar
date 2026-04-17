package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/pipeline/filter"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	shutdown := tracing.Init("jobradar-filter", cfg.JaegerEndpoint)
	defer shutdown()

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(context.Background(), cfg, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	worker := filter.NewWorker(consumer, producer, scorer, ledger,
		cfg.ScorerBatchSize, cfg.ScorerFlushTimeoutSeconds, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
