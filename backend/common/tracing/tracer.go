package tracing

import (
	"context"

	"github.com/trushilpatel/jobradar/common/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Init bootstraps the global OTel tracer provider and W3C propagator.
// endpoint is the OTLP HTTP endpoint, e.g. "http://jaeger:4318".
// If endpoint is empty, a no-op tracer is used and no export errors occur.
// Export failures are logged as structured warnings via log — never fatal.
// Returns a shutdown function that flushes pending spans — call with defer.
func Init(serviceName, endpoint string, log logger.Logger) func() {
	if endpoint == "" {
		return func() {}
	}

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn("tracing export failed (non-fatal)", logger.Err(err))
	}))

	ctx := context.Background()

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		panic("tracing: failed to create OTLP exporter: " + err.Error())
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		tp.Shutdown(context.Background()) //nolint:errcheck
	}
}
