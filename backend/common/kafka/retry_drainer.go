package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

// RetryDrainer consumes all uncommitted messages from a retry topic and
// republishes them to destTopic, stripping the next_attempt_at header so the
// worker processes them immediately.
type RetryDrainer interface {
	Drain(ctx context.Context, consumerGroupID, sourceTopic, destTopic string) (int64, error)
}

type retryDrainer struct {
	admin *kadm.Client
	cfg   config.KafkaConfig
}

// NewRetryDrainer creates a RetryDrainer backed by a Kafka admin client.
func NewRetryDrainer(cfg config.KafkaConfig) (RetryDrainer, error) {
	client, err := kgo.NewClient(brokerOpts(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("kafka retry drainer: create client: %w", err)
	}
	return &retryDrainer{admin: kadm.NewClient(client), cfg: cfg}, nil
}

func brokerOpts(cfg config.KafkaConfig) []kgo.Opt {
	opts := []kgo.Opt{kgo.SeedBrokers(strings.Split(cfg.Brokers, ",")...)}
	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{User: cfg.User, Pass: cfg.Password}.AsMechanism()))
	}
	return opts
}

// Drain reads all uncommitted messages in sourceTopic (from the committed
// offset of consumerGroupID to the current end offset) and publishes each to
// destTopic. The next_attempt_at header is stripped so workers process them
// immediately. Returns the number of records drained.
//
// For DLQ topics (no regular consumer group), pass a non-existent groupID
// (e.g. "jobradar-dlq-observer") — FetchOffsets returns zero, so all
// available messages are treated as unconsumed.
func (d *retryDrainer) Drain(ctx context.Context, consumerGroupID, sourceTopic, destTopic string) (int64, error) {
	endOffsets, err := d.admin.ListEndOffsets(ctx, sourceTopic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets for %q: %w", sourceTopic, err)
	}

	// Committed offsets tell us where to start. Errors treated as no commits
	// (start from earliest available offset).
	committed, _ := d.admin.FetchOffsets(ctx, consumerGroupID)

	type partRange struct{ end int64 }
	ranges := map[int32]partRange{}
	partOffsets := map[string]map[int32]kgo.Offset{sourceTopic: {}}
	var totalMessages int64

	for partition, endOff := range endOffsets[sourceTopic] {
		if endOff.Err != nil {
			continue
		}
		startAt := max(committed[sourceTopic][partition].At, 0)
		remaining := endOff.Offset - startAt
		if remaining <= 0 {
			continue
		}
		ranges[partition] = partRange{end: endOff.Offset}
		partOffsets[sourceTopic][partition] = kgo.NewOffset().At(startAt)
		totalMessages += remaining
	}

	if totalMessages == 0 {
		return 0, nil
	}

	// Direct partition assignment — does not interfere with existing retry consumers.
	consumer, err := kgo.NewClient(append(brokerOpts(d.cfg), kgo.ConsumePartitions(partOffsets))...)
	if err != nil {
		return 0, fmt.Errorf("create drain consumer: %w", err)
	}
	defer consumer.Close()

	publisher, err := kgo.NewClient(brokerOpts(d.cfg)...)
	if err != nil {
		return 0, fmt.Errorf("create drain publisher: %w", err)
	}
	defer publisher.Close()

	var drained int64
	for drained < totalMessages {
		fetches := consumer.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			return drained, fmt.Errorf("poll: %w", err)
		}

		var toPublish []*kgo.Record
		fetches.EachRecord(func(r *kgo.Record) {
			end, ok := ranges[r.Partition]
			if !ok || r.Offset >= end.end {
				return
			}
			headers := make([]kgo.RecordHeader, 0, len(r.Headers))
			for _, h := range r.Headers {
				if string(h.Key) != HeaderNextAttemptAt {
					headers = append(headers, h)
				}
			}
			toPublish = append(toPublish, &kgo.Record{
				Topic:   destTopic,
				Value:   r.Value,
				Headers: headers,
			})
			drained++
		})

		if len(toPublish) > 0 {
			if err := publisher.ProduceSync(ctx, toPublish...).FirstErr(); err != nil {
				return drained, fmt.Errorf("produce: %w", err)
			}
		}
	}

	return drained, nil
}
