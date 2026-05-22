package jetstream

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// Publisher sends messages to NATS JetStream subjects.
type Publisher interface {
	// Publish sends payload with an auto-generated Nats-Msg-Id.
	Publish(ctx context.Context, subject string, payload []byte) error
	// PublishWithMsgID sends payload with a caller-supplied Nats-Msg-Id for dedup.
	PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error
}

type jetStreamPublisher struct {
	js natsjs.JetStream
}

// NewPublisher returns a Publisher backed by the given JetStream context.
func NewPublisher(js natsjs.JetStream) Publisher {
	return &jetStreamPublisher{js: js}
}

func (p *jetStreamPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	return p.publish(ctx, subject, payload, uuid.New().String())
}

func (p *jetStreamPublisher) PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error {
	return p.publish(ctx, subject, payload, msgID)
}

func (p *jetStreamPublisher) publish(ctx context.Context, subject string, payload []byte, msgID string) error {
	if _, err := p.js.Publish(ctx, subject, payload, natsjs.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
