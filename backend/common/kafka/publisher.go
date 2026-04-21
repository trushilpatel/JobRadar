package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Publisher is the interface workers depend on to send messages to Kafka.
// The concrete Producer implements this — but workers never import Producer directly.
type Publisher interface {
	Publish(ctx context.Context, topic string, value []byte) error
	PublishWithHeaders(ctx context.Context, topic string, value []byte, headers []kgo.RecordHeader) error
}
