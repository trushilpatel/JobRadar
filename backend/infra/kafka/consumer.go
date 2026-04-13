package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/infra/logger"
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
		// the end of the topic — not the beginning. Workers are event-driven and
		// should not replay historical messages on first start.
		// Existing groups with committed offsets are unaffected by this setting.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
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
// Commits offsets after fn returns nil. Transient fetch errors are logged
// and skipped — only context cancellation stops the loop.
func (c *Consumer) Poll(ctx context.Context, fn func(topic string, value []byte) error) error {
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
			if err := fn(r.Topic, r.Value); err != nil {
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
