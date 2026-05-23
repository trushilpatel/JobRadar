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
	js "github.com/trushilpatel/jobradar/common/jetstream"
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

	shutdown := tracing.Init("jobradar-filter", cfg.Observability.JaegerEndpoint, log)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, consumer, publisher, rdb, err := initInfra(ctx, cfg, log)
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	worker, err := buildWorker(ctx, cfg, database, consumer, publisher, rdb, log)
	if err != nil {
		os.Exit(1)
	}

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}

func initInfra(ctx context.Context, cfg *config.Config, log logger.Logger) (*gorm.DB, *js.PullConsumer, js.Publisher, *redispkg.Client, error) {
	database, err := db.Open(cfg.DB, 5)
	if err != nil {
		log.Error("db open failed", err)
		return nil, nil, nil, nil, err
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.PoolReady(database) })

	njs, err := js.ConnectManaged(ctx, js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
	if err != nil {
		log.Error("nats connect failed", err)
		return nil, nil, nil, nil, err
	}

	if err := js.EnsureStreams(ctx, njs); err != nil {
		log.Error("nats ensure streams failed", err)
		return nil, nil, nil, nil, err
	}

	consumer, err := js.NewPullConsumer(ctx, njs, js.StreamJobsRaw, js.ConsumerFilterWorkers)
	if err != nil {
		log.Error("nats pull consumer failed", err)
		return nil, nil, nil, nil, err
	}

	publisher := js.NewPublisher(njs)

	rdb, err := redispkg.New(ctx, cfg.Redis.URL)
	if err != nil {
		log.Error("redis connect failed", err)
		return nil, nil, nil, nil, err
	}

	return database, consumer, publisher, rdb, nil
}

func buildWorker(ctx context.Context, cfg *config.Config, database *gorm.DB, consumer *js.PullConsumer, publisher js.Publisher, rdb *redispkg.Client, log logger.Logger) (*Worker, error) {
	rateStore, cooldownStore := llmstore.NewRedisStores(rdb, cfg.Cooldown)
	slots := llmcore.SlotsFromConfig(cfg.Filter.Slots)

	scorer, err := llmcore.NewScorer(ctx, llmcore.Options{
		Provider:       cfg.LLM.Provider,
		TargetRoles:    cfg.LLM.TargetRoles,
		YearsExp:       cfg.LLM.YearsExp,
		Log:            log,
		Slots:          slots,
		RateLimitStore: rateStore,
		CooldownStore:  cooldownStore,
	})
	if err != nil {
		log.Error("llm init failed", err)
		return nil, err
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	return NewWorker(consumer, publisher, scorer, ledger,
		cfg.Filter.BatchSize,
		time.Duration(cfg.Filter.FlushTimeoutSeconds)*time.Second,
		time.Duration(cfg.Filter.LLMCallIntervalSeconds)*time.Second,
		log), nil
}
