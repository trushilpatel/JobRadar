package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
)

// EnsureTopics creates the given topics if they don't already exist.
// Call this once at startup before any producer or consumer is used.
func EnsureTopics(ctx context.Context, cfg *config.Config, topics ...string) error {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.KafkaBrokers, ",")...),
	}

	if cfg.KafkaUser != "" && cfg.KafkaPassword != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.KafkaUser,
			Pass: cfg.KafkaPassword,
		}.AsMechanism()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("kafka admin: create client: %w", err)
	}
	defer client.Close()

	admin := kadm.NewClient(client)

	for _, topic := range topics {
		resp, err := admin.CreateTopics(ctx, 1, 1, nil, topic)
		if err != nil {
			return fmt.Errorf("kafka admin: create topic %q: %w", topic, err)
		}
		for _, detail := range resp {
			// TOPIC_ALREADY_EXISTS is fine — idempotent
			if detail.Err != nil && !isTopicExists(detail.Err.Error()) {
				return fmt.Errorf("kafka admin: topic %q: %w", topic, detail.Err)
			}
		}
	}

	return nil
}

func isTopicExists(errMsg string) bool {
	return strings.Contains(errMsg, "TOPIC_ALREADY_EXISTS") ||
		strings.Contains(errMsg, "already exists")
}
