package tracing

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// natsHeaderCarrier adapts nats.Header to the OTel TextMapCarrier interface.
type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string {
	vals := nats.Header(c).Values(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c natsHeaderCarrier) Set(key, val string) { nats.Header(c).Set(key, val) }

func (c natsHeaderCarrier) Keys() []string {
	h := nats.Header(c)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// InjectNATS injects the current span's W3C trace context into a nats.Header.
func InjectNATS(ctx context.Context, h nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(h))
}

// ExtractNATS extracts a W3C trace context from a nats.Header and returns
// a context carrying the remote span as parent.
func ExtractNATS(ctx context.Context, h nats.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, natsHeaderCarrier(h))
}

// compile-time assertion that natsHeaderCarrier satisfies propagation.TextMapCarrier
var _ propagation.TextMapCarrier = natsHeaderCarrier{}
