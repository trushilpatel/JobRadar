package kafka

import (
	"context"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type Consumer struct {
	client *kgo.Client
	log    logger.Logger
}

// NewConsumer creates a Kafka consumer for the given group and topics.
// fetchMaxWait controls how long the broker waits before returning a fetch with
// fewer than FetchMinBytes. Pass 0 to use the Kafka default (5 seconds).
// Set a longer duration (e.g. 2 minutes) to accumulate jobs at the broker
// before processing, reducing LLM calls during trickle traffic.
func NewConsumer(cfg config.KafkaConfig, log logger.Logger, groupID string, fetchMaxWait time.Duration, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		// For a brand-new consumer group with no committed offsets, start from
		// the beginning — process all pending messages before following new ones.
		// Existing groups with committed offsets are unaffected by this setting.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	if fetchMaxWait > 0 {
		opts = append(opts, kgo.FetchMaxWait(fetchMaxWait))
	}

	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Consumer{client: client, log: log}, nil
}

// Poll fetches the next batch of records and calls fn for each one.
// fn receives the Kafka record headers so workers can extract trace context via tracing.ExtractKafka.
// Commits offsets after fn returns nil. Transient fetch errors are logged and skipped.
func (c *Consumer) Poll(ctx context.Context, fn func(topic string, value []byte, headers []kgo.RecordHeader) error) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fetches.Err(); err != nil {
			c.log.Warn("fetch error, retrying", logger.Err(err))
			time.Sleep(time.Second)
			continue
		}

		var lastErr error
		fetches.EachRecord(func(r *kgo.Record) {
			if err := fn(r.Topic, r.Value, r.Headers); err != nil {
				lastErr = err
			}
		})

		if lastErr != nil {
			return lastErr
		}

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			return err
		}
	}
}

// PollBatch fetches the next batch of records and calls fn with all records at once.
// fn must return nil only after all processing for the batch is complete — offsets are
// committed after fn returns, guaranteeing at-least-once delivery even with async work.
func (c *Consumer) PollBatch(ctx context.Context, fn func(records []*kgo.Record) error) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fetches.Err(); err != nil {
			c.log.Warn("fetch error, retrying", logger.Err(err))
			time.Sleep(time.Second)
			continue
		}

		var records []*kgo.Record
		fetches.EachRecord(func(r *kgo.Record) {
			records = append(records, r)
		})

		if len(records) == 0 {
			continue
		}

		if err := fn(records); err != nil {
			return err
		}

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			return err
		}
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}
