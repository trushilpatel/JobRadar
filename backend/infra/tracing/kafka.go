package tracing

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
)

// kafkaCarrier adapts []kgo.RecordHeader to propagation.TextMapCarrier
// so the W3C traceparent can be injected into and extracted from Kafka messages.
type kafkaCarrier []kgo.RecordHeader

func (c kafkaCarrier) Get(key string) string {
	for _, h := range c {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaCarrier) Set(key, value string) {
	*c = append(*c, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c kafkaCarrier) Keys() []string {
	keys := make([]string, len(c))
	for i, h := range c {
		keys[i] = h.Key
	}
	return keys
}

// InjectKafka injects the W3C traceparent from ctx into headers and returns
// the updated slice. Call before Producer.Publish to carry the trace across services.
func InjectKafka(ctx context.Context, headers []kgo.RecordHeader) []kgo.RecordHeader {
	carrier := kafkaCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	return []kgo.RecordHeader(carrier)
}

// ExtractKafka reads the W3C traceparent from Kafka message headers and returns
// a ctx with the remote span set as the parent. Call after Poll delivers a record.
func ExtractKafka(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	carrier := kafkaCarrier(headers)
	return otel.GetTextMapPropagator().Extract(ctx, &carrier)
}
