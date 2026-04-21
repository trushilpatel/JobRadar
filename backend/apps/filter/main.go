package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
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
	"github.com/twmb/franz-go/pkg/kgo"
)

const mainConsumerGroupID = "job-listing-relevance-filter"

// maxRetrySleep caps the blocking sleep used by retry consumers when waiting for
// next_attempt_at. 1h matches the longest tier; anything longer means the header
// was malformed, and we'd rather not block the group heartbeat forever.
const maxRetrySleep = time.Hour

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

	// Retry consumers forward due messages back to the main topic. One per tier.
	retryInputs := []struct {
		topic   string
		groupID string
	}{
		{kafka.TopicJobListingsRawRetry1m, mainConsumerGroupID + "-retry-1m"},
		{kafka.TopicJobListingsRawRetry10m, mainConsumerGroupID + "-retry-10m"},
		{kafka.TopicJobListingsRawRetry1h, mainConsumerGroupID + "-retry-1h"},
	}

	retryConsumers := make([]*kafka.Consumer, 0, len(retryInputs))
	for _, ri := range retryInputs {
		c, err := kafka.NewConsumer(cfg.Kafka, log, ri.groupID, ri.topic)
		if err != nil {
			log.Error("retry consumer init failed", err, logger.Str("topic", ri.topic))
			os.Exit(1)
		}
		retryConsumers = append(retryConsumers, c)
	}
	defer func() {
		for _, c := range retryConsumers {
			c.Close()
		}
	}()

	group, gctx := errgroup.WithContext(ctx)
	group.Go(func() error { return worker.Run(gctx) })
	for i, ri := range retryInputs {
		c := retryConsumers[i]
		inputTopic := ri.topic
		group.Go(func() error { return runRetryConsumer(gctx, c, producer, inputTopic, log) })
	}

	if err := group.Wait(); err != nil && ctx.Err() == nil {
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

	if err := kafka.EnsureTopics(ctx, cfg.Kafka,
		kafka.TopicJobListingsRaw,
		kafka.TopicJobListingsFiltered,
		kafka.TopicJobListingsRawRetry1m,
		kafka.TopicJobListingsRawRetry10m,
		kafka.TopicJobListingsRawRetry1h,
		kafka.TopicJobListingsRawDLQ,
	); err != nil {
		log.Error("kafka topics failed", err)
		return nil, nil, nil, nil, err
	}

	consumer, err := kafka.NewConsumer(cfg.Kafka, log, mainConsumerGroupID, kafka.TopicJobListingsRaw)
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
	})
	if err != nil {
		log.Error("llm init failed", err)
		return nil, err
	}

	ledger := jobsvc.NewSeenLedgerStore(database)
	return NewWorker(consumer, producer, scorer, ledger,
		cfg.Filter.BatchSize, cfg.Filter.FlushTimeoutSeconds, cfg.Filter.LLMParallelism, log), nil
}

// runRetryConsumer consumes one retry-tier topic, sleeps until each record's
// next_attempt_at, then re-publishes the record's value back onto the main
// job_listings.raw topic preserving its retry_count header. Returning nil from
// the Poll callback commits the retry-tier offset; the main worker will pick
// the message up on its next poll.
func runRetryConsumer(ctx context.Context, consumer *kafka.Consumer, producer kafka.Publisher, inputTopic string, log logger.Logger) error {
	log.Info("retry consumer started", logger.Str("topic", inputTopic))
	return consumer.Poll(ctx, func(_ string, value []byte, headers []kgo.RecordHeader) error {
		nextAt := kafka.NextAttemptAtFromHeaders(headers)
		if !nextAt.IsZero() {
			wait := time.Until(nextAt)
			if wait > maxRetrySleep {
				wait = maxRetrySleep
			}
			if wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		retryCount := kafka.RetryCountFromHeaders(headers)
		// Preserve retry_count; next_attempt_at is irrelevant on the main topic but
		// harmless. We re-use BuildRetryHeaders so both stay attached.
		outHeaders := kafka.BuildRetryHeaders(retryCount, nextAt)

		// Guard against a malformed payload — shouldn't happen, but if it does,
		// skip rather than loop forever. Parsing is cheap.
		var probe json.RawMessage
		if err := json.Unmarshal(value, &probe); err != nil {
			log.Warn("retry consumer: malformed payload, dropping",
				logger.Str("topic", inputTopic), logger.Err(err))
			return nil
		}

		if err := producer.PublishWithHeaders(ctx, kafka.TopicJobListingsRaw, value, outHeaders); err != nil {
			log.Error("retry consumer: republish failed", err,
				logger.Str("topic", inputTopic),
				logger.Int("retry_count", retryCount))
			return err
		}
		return nil
	})
}
