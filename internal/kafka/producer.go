package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/trushilpatel/jobradar/config"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(cfg *config.Config) (*Producer, error) {
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
		return nil, err
	}

	return &Producer{client: client}, nil
}

func (p *Producer) Publish(ctx context.Context, topic string, value []byte) error {
	record := &kgo.Record{
		Topic: topic,
		Value: value,
	}

	return p.client.ProduceSync(ctx, record).FirstErr()
}

func (p *Producer) Close() {
	p.client.Close()
}
