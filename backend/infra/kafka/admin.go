package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

var ensureTopicsBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}

// EnsureTopics creates the given topics if they don't already exist.
// Retries with backoff to tolerate Kafka finishing its startup after the
// healthcheck has already passed.
// Call this once at startup before any producer or consumer is used.
func EnsureTopics(ctx context.Context, cfg config.KafkaConfig, topics ...string) error {
	var lastErr error
	for i, delay := range append([]time.Duration{0}, ensureTopicsBackoff...) {
		if i > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if lastErr = ensureTopicsOnce(ctx, cfg, topics...); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func ensureTopicsOnce(ctx context.Context, cfg config.KafkaConfig, topics ...string) error {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...),
	}

	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{
			User: cfg.User,
			Pass: cfg.Password,
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
