package kafka

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type Consumer struct {
	client *kgo.Client
	log    logger.Logger
}

func NewConsumer(cfg *config.Config, log logger.Logger, groupID string, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.KafkaBrokers, ",")...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		// For a brand-new consumer group with no committed offsets, start from
		// the beginning — process all pending messages before following new ones.
		// Existing groups with committed offsets are unaffected by this setting.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	if cfg.KafkaUser != "" && cfg.KafkaPassword != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.KafkaUser,
			Pass: cfg.KafkaPassword,
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

func (c *Consumer) Close() {
	c.client.Close()
}
