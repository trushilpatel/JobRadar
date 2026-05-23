package jetstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// Msg wraps a JetStream message with explicit ack/nak/progress control.
type Msg struct {
	inner natsjs.Msg
}

func (m *Msg) Data() []byte                       { return m.inner.Data() }
func (m *Msg) Subject() string                    { return m.inner.Subject() }
func (m *Msg) Ack() error                         { return m.inner.Ack() }
func (m *Msg) Nak() error                         { return m.inner.Nak() }
func (m *Msg) NakWithDelay(d time.Duration) error { return m.inner.NakWithDelay(d) }
func (m *Msg) InProgress() error                  { return m.inner.InProgress() }
func (m *Msg) Term() error                        { return m.inner.Term() }

// Headers returns the raw NATS message headers.
// Use tracing.ExtractNATS(ctx, msg.Headers()) to extract W3C trace context from a consumed message.
func (m *Msg) Headers() nats.Header { return m.inner.Headers() }

// DeliveryCount returns how many times NATS has delivered this message.
// First delivery = 1, first retry = 2, etc. Returns 1 on metadata error.
func (m *Msg) DeliveryCount() int {
	meta, err := m.inner.Metadata()
	if err != nil || meta == nil {
		return 1
	}
	return int(meta.NumDelivered)
}

// PullConsumer fetches messages from a durable NATS JetStream consumer.
type PullConsumer struct {
	consumer natsjs.Consumer
}

// NewPullConsumer binds to an existing durable consumer on the given stream.
// The consumer must already exist (created via EnsureStreams).
func NewPullConsumer(ctx context.Context, js natsjs.JetStream, stream, consumer string) (*PullConsumer, error) {
	c, err := js.Consumer(ctx, stream, consumer)
	if err != nil {
		return nil, fmt.Errorf("bind consumer %s/%s: %w", stream, consumer, err)
	}
	return &PullConsumer{consumer: c}, nil
}

// Fetch retrieves up to n messages, waiting at most maxWait for them to arrive.
// Returns nil, nil when no messages are available within maxWait — callers should loop.
func (c *PullConsumer) Fetch(n int, maxWait time.Duration) ([]*Msg, error) {
	// FetchHeartbeat keeps the long-poll alive on cloud brokers (e.g. Synadia Cloud)
	// that enforce idle heartbeats. 5s is well under any cloud provider's idle timeout.
	result, err := c.consumer.Fetch(n,
		natsjs.FetchMaxWait(maxWait),
		natsjs.FetchHeartbeat(5*time.Second),
	)
	if err != nil {
		if errors.Is(err, natsjs.ErrNoMessages) || errors.Is(err, natsjs.ErrNoHeartbeat) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch: %w", err)
	}
	var msgs []*Msg
	for msg := range result.Messages() {
		msgs = append(msgs, &Msg{inner: msg})
	}
	if err := result.Error(); err != nil {
		if errors.Is(err, natsjs.ErrNoMessages) || errors.Is(err, natsjs.ErrNoHeartbeat) {
			return msgs, nil
		}
		return msgs, fmt.Errorf("fetch error: %w", err)
	}
	return msgs, nil
}

// FetchOne retrieves exactly one message, waiting at most maxWait.
// Returns nil, nil when no message arrives within maxWait.
func (c *PullConsumer) FetchOne(ctx context.Context, maxWait time.Duration) (*Msg, error) {
	msgs, err := c.Fetch(1, maxWait)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	return msgs[0], nil
}
