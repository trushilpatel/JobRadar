package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
)

type Consumer struct {
	client *kgo.Client
}

func NewConsumer(cfg *config.Config, groupID string, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.KafkaBrokers, ",")...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
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

	return &Consumer{client: client}, nil
}

// Poll fetches the next batch of records and calls fn for each one.
// Commits offsets after fn returns nil.
func (c *Consumer) Poll(ctx context.Context, fn func(topic string, value []byte) error) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			return err
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
