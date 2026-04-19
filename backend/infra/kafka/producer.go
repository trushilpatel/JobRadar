package kafka

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(cfg config.KafkaConfig) (*Producer, error) {
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
		return nil, err
	}

	return &Producer{client: client}, nil
}

func (p *Producer) Publish(ctx context.Context, topic string, value []byte) error {
	record := &kgo.Record{
		Topic:   topic,
		Value:   value,
		Headers: tracing.InjectKafka(ctx, nil),
	}
	return p.client.ProduceSync(ctx, record).FirstErr()
}

func (p *Producer) Close() {
	p.client.Close()
}
