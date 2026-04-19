package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Start begins a named span as a child of whatever span is in ctx.
// The returned ctx carries the new span — pass it to all downstream calls.
//
// Pair with End using a named error return:
//
//	func Foo(ctx context.Context) (err error) {
//	    ctx, span := tracing.Start(ctx, "pkg.Foo")
//	    defer tracing.End(span, &err)
//	    ...
//	}
func Start(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer("jobradar").Start(ctx, name)
}

// End closes span and records the error if non-nil.
// Pass a pointer to the function's named return error so defer captures the final value.
func End(span trace.Span, errPtr *error) {
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
	}
	span.End()
}
