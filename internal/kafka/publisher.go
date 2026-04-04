package kafka

import "context"

// Publisher is the interface workers depend on to send messages to Kafka.
// The concrete Producer implements this — but workers never import Producer directly.
type Publisher interface {
	Publish(ctx context.Context, topic string, value []byte) error
}
