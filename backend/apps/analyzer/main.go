package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
	log := logger.Init("enricher")

	shutdown := tracing.Init("jobradar-enricher", cfg.Observability.JaegerEndpoint)
	defer shutdown()

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

	scorer, err := llmcore.NewScorer(llmcore.Options{
		Provider:    cfg.LLM.Provider,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.Enricher.Model,
		TargetRoles: cfg.LLM.TargetRoles,
		YearsExp:    cfg.LLM.YearsExp,
		Log:         log,
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
