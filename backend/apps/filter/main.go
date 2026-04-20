package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	redispkg "github.com/trushilpatel/jobradar/common/redis"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
	llmstore "github.com/trushilpatel/jobradar/service/llm/store"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	shutdown := tracing.Init("jobradar-filter", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, consumer, producer, rdb, err := initInfra(ctx, cfg, log)
	if err != nil {
		os.Exit(1)
	}
	defer consumer.Close()
	defer producer.Close()
	defer func() { _ = rdb.Close() }()

	worker, err := buildWorker(ctx, cfg, database, consumer, producer, rdb, log)
	if err != nil {
		os.Exit(1)
	}

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}

func initInfra(ctx context.Context, cfg *config.Config, log logger.Logger) (*gorm.DB, *kafka.Consumer, *kafka.Producer, *redispkg.Client, error) {
	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		return nil, nil, nil, nil, err
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(ctx, cfg.Kafka, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		return nil, nil, nil, nil, err
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		return nil, nil, nil, nil, err
	}

	producer, err := kafka.NewProducer(cfg.Kafka)
	if err != nil {
		log.Error("kafka producer failed", err)
		return nil, nil, nil, nil, err
	}

	rdb, err := redispkg.New(ctx, cfg.Redis.Addr, cfg.Redis.DB)
	if err != nil {
		log.Error("redis connect failed", err)
		return nil, nil, nil, nil, err
	}

	return database, consumer, producer, rdb, nil
}

func buildWorker(ctx context.Context, cfg *config.Config, database *gorm.DB, consumer *kafka.Consumer, producer *kafka.Producer, rdb *redispkg.Client, log logger.Logger) (*Worker, error) {
	rateStore := llmstore.NewLLMRateLimitRedisStore(rdb)
	cooldownStore := llmstore.NewLLMCooldownRedisStore(rdb, llmstore.CooldownPolicy{
		Base:   time.Duration(cfg.Cooldown.BaseSeconds) * time.Second,
		Max:    time.Duration(cfg.Cooldown.MaxSeconds) * time.Second,
		Jitter: cfg.Cooldown.Jitter,
	})

	slots := make([]llmcore.SlotOptions, 0, len(cfg.Filter.Slots))
	for _, s := range cfg.Filter.Slots {
		slots = append(slots, llmcore.SlotOptions{
			KeyID:  s.KeyID,
			APIKey: s.APIKey,
			Model:  s.Model,
			RPM:    s.RPM,
			RPD:    s.RPD,
			TPMIn:  s.TPMIn,
		})
	}

	scorer, err := llmcore.NewScorer(ctx, llmcore.Options{
		Provider:       cfg.LLM.Provider,
		TargetRoles:    cfg.LLM.TargetRoles,
		YearsExp:       cfg.LLM.YearsExp,
		Log:            log,
		Slots:          slots,
		RateLimitStore: rateStore,
		CooldownStore:  cooldownStore,
		// Legacy fallback — used when Slots is empty.
		APIKey: cfg.LLM.APIKey,
		Model:  cfg.Filter.Model,
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
