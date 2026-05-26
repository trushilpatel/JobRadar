package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	log := logger.Init("enricher")

	shutdown := tracing.Init("jobradar-enricher", cfg.Observability.OTelEndpoint, log)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DB, 5)
	if err != nil {
		log.Error("db open failed", err)
		shutdown()
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.PoolReady(database) })

	njs, err := js.ConnectManaged(ctx, js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
	if err != nil {
		log.Error("nats connect failed", err)
		shutdown()
		os.Exit(1)
	}

	if err := js.EnsureStreams(ctx, njs); err != nil {
		log.Error("nats ensure streams failed", err)
		shutdown()
		os.Exit(1)
	}

	consumer, err := js.NewPullConsumer(ctx, njs, js.StreamJobsFiltered, js.ConsumerAnalyzerWorkers)
	if err != nil {
		log.Error("nats pull consumer failed", err)
		shutdown()
		os.Exit(1)
	}

	publisher := js.NewPublisher(njs)

	rdb, err := redispkg.New(ctx, cfg.Redis.URL)
	if err != nil {
		log.Error("redis connect failed", err)
		shutdown()
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	rateStore, cooldownStore := llmstore.NewRedisStores(rdb, cfg.Cooldown)
	slots := llmcore.SlotsFromConfig(cfg.Enricher.Slots)

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
		shutdown()
		os.Exit(1)
	}

	jobStore := jobsvc.NewJobStore(database)
	ledgerStore := jobsvc.NewSeenLedgerStore(database)
	snapshotStore := jobsvc.NewSnapshotStore(database)
	techTagStore := jobsvc.NewTechTagStore(database)
	pipeline := jobsvc.NewPipelineService(jobStore, ledgerStore, snapshotStore, techTagStore, log)

	rateLimiter := NewAdapterRateLimiter(
		cfg.Enricher.DefaultRPS,
		cfg.Enricher.DefaultBurst,
		time.Duration(cfg.Enricher.JitterMs)*time.Millisecond,
		log,
	)

	worker := NewWorker(WorkerOptions{
		Consumer:         consumer,
		Publisher:        publisher,
		Scorer:           scorer,
		Pipeline:         pipeline,
		Ledger:           ledgerStore,
		RateLimiter:      rateLimiter,
		Log:              log,
		MaxExpYears:      cfg.Enricher.MaxExpYears,
		FetchConcurrency: cfg.Enricher.FetchConcurrency,
		LLMBatchSize:     cfg.Enricher.LLMBatchSize,
		LLMCallInterval:  time.Duration(cfg.Enricher.LLMCallIntervalSeconds) * time.Second,
		FlushTimeout:     time.Duration(cfg.Enricher.FlushTimeoutSeconds) * time.Second,
	})

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		shutdown()
		os.Exit(1)
	}
	log.Info("shutting down")
}
