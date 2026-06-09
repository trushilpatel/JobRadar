package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

const tracerName = "crawler-v2"

// Init bootstraps the global OTel tracer provider and W3C propagator.
// endpoint is the OTLP HTTP endpoint, e.g. "http://obs-alloy.svc.cluster.local:4318".
// If endpoint is empty a no-op tracer is used — no export errors occur.
// Export failures are logged as warnings — never fatal.
// Returns a shutdown function that flushes pending spans — always call with defer.
func Init(serviceName, endpoint string, log logger.Logger) func() {
	if endpoint == "" {
		return func() {}
	}

	var exportWarnOnce sync.Once
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		exportWarnOnce.Do(func() {
			log.Warn("tracing export failed (non-fatal) — further failures suppressed", logger.Err(err))
		})
	}))

	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		panic("telemetry: failed to create OTLP exporter: " + err.Error())
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

// Start begins a named span as a child of whatever span is in ctx.
// The returned ctx carries the new span — pass it to all downstream calls.
//
// Pair with End using a named error return:
//
//	func Foo(ctx context.Context) (_ Result, err error) {
//	    ctx, span := telemetry.Start(ctx, "greenhouse.FetchJobListing")
//	    defer telemetry.End(span, &err)
//	    ...
//	}
func Start(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name)
}

// End closes the span and records the error if non-nil.
// Pass a pointer to the function's named return error so defer captures the final value.
func End(span trace.Span, errPtr *error) {
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
	}
	span.End()
}
