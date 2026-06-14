package jetstream

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/trushilpatel/jobradar/common/tracing"
)

// Publisher sends messages to NATS JetStream subjects.
type Publisher interface {
	// Publish sends payload with an auto-generated Nats-Msg-Id.
	Publish(ctx context.Context, subject string, payload []byte) error
	// PublishWithMsgID sends payload with a caller-supplied Nats-Msg-Id for dedup.
	PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error
	// PublishWithHeaders sends payload and merges the supplied headers into the
	// NATS message alongside the auto-generated Nats-Msg-Id and the OTel W3C
	// trace headers. Keys already present in headers take precedence over defaults.
	PublishWithHeaders(ctx context.Context, subject string, payload []byte, headers map[string]string) error
}

type jetStreamPublisher struct {
	js natsjs.JetStream
}

// NewPublisher returns a Publisher backed by the given JetStream context.
func NewPublisher(js natsjs.JetStream) Publisher {
	return &jetStreamPublisher{js: js}
}

func (p *jetStreamPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	return p.publish(ctx, subject, payload, uuid.New().String(), nil)
}

func (p *jetStreamPublisher) PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error {
	return p.publish(ctx, subject, payload, msgID, nil)
}

func (p *jetStreamPublisher) PublishWithHeaders(ctx context.Context, subject string, payload []byte, headers map[string]string) error {
	return p.publish(ctx, subject, payload, uuid.New().String(), headers)
}

func (p *jetStreamPublisher) publish(ctx context.Context, subject string, payload []byte, msgID string, extraHeaders map[string]string) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    payload,
		Header:  make(nats.Header),
	}
	tracing.InjectNATS(ctx, msg.Header)
	// Apply extra headers after trace injection so caller values take precedence.
	for k, v := range extraHeaders {
		msg.Header.Set(k, v)
	}
	if _, err := p.js.PublishMsg(ctx, msg, natsjs.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
