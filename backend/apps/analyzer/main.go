package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
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
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/sync/errgroup"
)

func main() {
	cfg := config.Load()
	log := logger.Init("enricher")

	shutdown := tracing.Init("jobradar-enricher", cfg.Observability.JaegerEndpoint)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DB, 5)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })

	// Ensure analyzer-owned retry + DLQ topics exist before any producer/consumer
	// connects. The main .filtered topic is created by the filter worker — do not
	// include it here.
	if err := kafka.EnsureTopics(ctx, cfg.Kafka,
		kafka.TopicJobListingsFilteredRetry1m,
		kafka.TopicJobListingsFilteredRetry10m,
		kafka.TopicJobListingsFilteredRetry1h,
		kafka.TopicJobListingsFilteredDLQ,
	); err != nil {
		log.Error("ensure topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, "job-post-analyzer",
		time.Duration(cfg.Enricher.FlushTimeoutSeconds)*time.Second,
		kafka.TopicJobListingsFiltered)
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

	rateLimiter := NewAdapterRateLimiter(
		cfg.Enricher.DefaultRPS,
		cfg.Enricher.DefaultBurst,
		time.Duration(cfg.Enricher.JitterMs)*time.Millisecond,
		log,
	)

	worker := NewWorker(WorkerOptions{
		Consumer:         consumer,
		Publisher:        producer,
		Scorer:           scorer,
		Pipeline:         pipeline,
		Ledger:           ledgerStore,
		RateLimiter:      rateLimiter,
		Log:              log,
		MaxExpYears:      cfg.Enricher.MaxExpYears,
		FetchConcurrency: cfg.Enricher.FetchConcurrency,
		LLMBatchSize:     cfg.Enricher.LLMBatchSize,
		LLMCallInterval:  time.Duration(cfg.Enricher.LLMCallIntervalSeconds) * time.Second,
	})

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return worker.Run(gctx) })

	for _, rt := range []struct {
		topic   string
		groupID string
	}{
		{kafka.TopicJobListingsFilteredRetry1m, "job-post-analyzer-retry-1m"},
		{kafka.TopicJobListingsFilteredRetry10m, "job-post-analyzer-retry-10m"},
		{kafka.TopicJobListingsFilteredRetry1h, "job-post-analyzer-retry-1h"},
	} {
		rt := rt
		g.Go(func() error {
			return runRetryConsumer(gctx, cfg.Kafka, log, producer, rt.topic, rt.groupID)
		})
	}

	if err := g.Wait(); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}

// runRetryConsumer consumes from a retry tier topic, waits until each message's
// scheduled next_attempt_at, and republishes the payload back onto the main
// .filtered topic so the analyzer worker picks it up again. The retry_count
// header is preserved so the main worker knows the current attempt count.
func runRetryConsumer(
	ctx context.Context,
	kcfg config.KafkaConfig,
	log logger.Logger,
	publisher kafka.Publisher,
	inputTopic string,
	groupID string,
) error {
	retryLog := log.With(logger.Str("retry_topic", inputTopic))
	consumer, err := kafka.NewConsumer(kcfg, log, groupID, 0, inputTopic)
	if err != nil {
		return fmt.Errorf("retry consumer %s: %w", inputTopic, err)
	}
	defer consumer.Close()

	retryLog.Info("retry consumer started", logger.Str("group_id", groupID))

	return consumer.Poll(ctx, func(_ string, value []byte, headers []kgo.RecordHeader) error {
		nextAt := kafka.NextAttemptAtFromHeaders(headers)
		if wait := time.Until(nextAt); wait > 0 {
			// Cap the blocking sleep at 1h — matches the longest retry tier and
			// keeps startup drains bounded if the clock is wildly off.
			if wait > time.Hour {
				wait = time.Hour
			}
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}

		retryCount := kafka.RetryCountFromHeaders(headers)
		// Preserve the existing retry_count header so the main worker knows
		// the current attempt on re-arrival. next_attempt_at is no longer
		// relevant once the message is back on the primary topic.
		outHeaders := []kgo.RecordHeader{
			{Key: kafka.HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		}
		if err := publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsFiltered, value, outHeaders); err != nil {
			retryLog.Error("republish to main topic failed", err)
			return fmt.Errorf("republish %s: %w", inputTopic, err)
		}
		retryLog.Debug("republished",
			logger.Int("retry_count", retryCount),
		)
		return nil
	})
}
