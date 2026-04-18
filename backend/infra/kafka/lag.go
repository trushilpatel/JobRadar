package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

// LagReader returns the consumer group lag for a given group and topic.
type LagReader interface {
	GetLag(ctx context.Context, groupID, topic string) (int64, error)
}

type lagReader struct {
	admin *kadm.Client
}

// NewLagReader creates a long-lived Kafka admin client for lag queries.
func NewLagReader(cfg config.KafkaConfig) (LagReader, error) {
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
		return nil, fmt.Errorf("kafka lag reader: create client: %w", err)
	}
	return &lagReader{admin: kadm.NewClient(client)}, nil
}

// GetLag returns the total unconsumed message count for groupID on topic.
// Returns 0 with no error if the group has no committed offsets yet.
func (r *lagReader) GetLag(ctx context.Context, groupID, topic string) (int64, error) {
	committed, err := r.admin.FetchOffsets(ctx, groupID)
	if err != nil {
		return 0, fmt.Errorf("fetch offsets for group %q: %w", groupID, err)
	}

	endOffsets, err := r.admin.ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets for topic %q: %w", topic, err)
	}

	var total int64
	for partition, endOff := range endOffsets[topic] {
		if endOff.Err != nil {
			continue
		}
		committedAt := committed[topic][partition].At
		if committedAt < 0 {
			committedAt = 0
		}
		if lag := endOff.Offset - committedAt; lag > 0 {
			total += lag
		}
	}
	return total, nil
}
