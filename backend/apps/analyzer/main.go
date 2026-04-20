package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	log := logger.Init("enricher")

	shutdown := tracing.Init("jobradar-enricher", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	rdb, err := redispkg.New(ctx, cfg.Redis.Addr, cfg.Redis.DB)
	if err != nil {
		log.Error("redis connect failed", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	rateStore := llmstore.NewLLMRateLimitRedisStore(rdb)
	cooldownStore := llmstore.NewLLMCooldownRedisStore(rdb, llmstore.CooldownPolicy{
		Base:   time.Duration(cfg.Cooldown.BaseSeconds) * time.Second,
		Max:    time.Duration(cfg.Cooldown.MaxSeconds) * time.Second,
		Jitter: cfg.Cooldown.Jitter,
	})

	slots := make([]llmcore.SlotOptions, 0, len(cfg.Enricher.Slots))
	for _, s := range cfg.Enricher.Slots {
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
		Model:  cfg.Enricher.Model,
		Limits: llmcore.RateLimits{
			RPM:   cfg.Enricher.RateLimits.RPM,
			RPD:   cfg.Enricher.RateLimits.RPD,
			TPMIn: cfg.Enricher.RateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	jobStore := jobsvc.NewJobStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	snapshotStore := jobsvc.NewSnapshotStore(database)
	techTagStore := jobsvc.NewTechTagStore(database)
	pipeline := jobsvc.NewPipelineService(jobStore, ledgerStore, snapshotStore, techTagStore)
	worker := NewWorker(consumer, scorer, pipeline, log, cfg.Enricher.MaxExpYears, cfg.Enricher.Concurrency)

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
