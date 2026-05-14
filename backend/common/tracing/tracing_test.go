package tracing_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupRecorder registers a SpanRecorder against the global tracer provider so
// spans created via tracing.Start are captured. It restores the previous global
// provider and propagator on test cleanup so tests stay isolated.
func setupRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})
	return rec
}

func TestStart_CreatesChildSpanInContext(t *testing.T) {
	rec := setupRecorder(t)

	ctx, span := tracing.Start(context.Background(), "pkg.Op")
	if !span.SpanContext().IsValid() {
		t.Fatal("Start: span context is not valid")
	}
	if trace.SpanFromContext(ctx).SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Fatal("Start: returned ctx does not carry the new span")
	}
	span.End()

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].Name() != "pkg.Op" {
		t.Fatalf("expected span name 'pkg.Op', got %q", ended[0].Name())
	}
}

func TestStart_NestedSpanIsChildOfParent(t *testing.T) {
	rec := setupRecorder(t)

	ctx, parent := tracing.Start(context.Background(), "parent")
	_, child := tracing.Start(ctx, "child")
	child.End()
	parent.End()

	ended := rec.Ended()
	if len(ended) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(ended))
	}

	// First ended is the child; parent ends second.
	childSpan := ended[0]
	parentSpan := ended[1]
	if childSpan.Parent().SpanID() != parentSpan.SpanContext().SpanID() {
		t.Fatalf("child parent mismatch: child.parent=%v parent.id=%v",
			childSpan.Parent().SpanID(), parentSpan.SpanContext().SpanID())
	}
}

func TestEnd_NilErrorPointerLeavesSpanUnset(t *testing.T) {
	rec := setupRecorder(t)

	_, span := tracing.Start(context.Background(), "noerr")
	tracing.End(span, nil)

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].Status().Code != codes.Unset {
		t.Fatalf("expected status Unset, got %v", ended[0].Status().Code)
	}
	if len(ended[0].Events()) != 0 {
		t.Fatalf("expected no events, got %d", len(ended[0].Events()))
	}
}

func TestEnd_NilDereferencedErrorLeavesSpanUnset(t *testing.T) {
	rec := setupRecorder(t)

	_, span := tracing.Start(context.Background(), "noerr-ptr")
	var err error
	tracing.End(span, &err)

	ended := rec.Ended()
	if ended[0].Status().Code != codes.Unset {
		t.Fatalf("expected status Unset for nil-deref err, got %v", ended[0].Status().Code)
	}
	if len(ended[0].Events()) != 0 {
		t.Fatalf("expected no events, got %d", len(ended[0].Events()))
	}
}

func TestEnd_RecordsErrorAndSetsErrorStatus(t *testing.T) {
	rec := setupRecorder(t)

	_, span := tracing.Start(context.Background(), "with-err")
	err := errors.New("boom")
	tracing.End(span, &err)

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	got := ended[0]
	if got.Status().Code != codes.Error {
		t.Fatalf("expected status Error, got %v", got.Status().Code)
	}
	if got.Status().Description != "boom" {
		t.Fatalf("expected description 'boom', got %q", got.Status().Description)
	}
	if len(got.Events()) == 0 {
		t.Fatal("expected RecordError to add at least one event")
	}
}

func TestInjectKafka_ExtractKafka_RoundTrip(t *testing.T) {
	setupRecorder(t)

	// Build a context that carries an active span (so there's something to propagate).
	ctx, span := tracing.Start(context.Background(), "producer")
	defer span.End()
	parentSC := span.SpanContext()

	headers := tracing.InjectKafka(ctx, nil)
	if len(headers) == 0 {
		t.Fatal("InjectKafka: expected headers to be appended, got none")
	}

	// traceparent header must be present.
	var foundTraceparent bool
	for _, h := range headers {
		if h.Key == "traceparent" {
			foundTraceparent = true
		}
	}
	if !foundTraceparent {
		t.Fatalf("InjectKafka: traceparent header missing, headers=%+v", headers)
	}

	// Extract into a fresh context — the remote span must match the original.
	extractedCtx := tracing.ExtractKafka(context.Background(), headers)
	remoteSC := trace.SpanContextFromContext(extractedCtx)
	if !remoteSC.IsValid() {
		t.Fatal("ExtractKafka: remote span context is not valid")
	}
	if remoteSC.TraceID() != parentSC.TraceID() {
		t.Fatalf("TraceID mismatch: want %v got %v", parentSC.TraceID(), remoteSC.TraceID())
	}
	if remoteSC.SpanID() != parentSC.SpanID() {
		t.Fatalf("SpanID mismatch: want %v got %v", parentSC.SpanID(), remoteSC.SpanID())
	}
}

func TestInjectKafka_PreservesExistingHeaders(t *testing.T) {
	setupRecorder(t)

	ctx, span := tracing.Start(context.Background(), "producer")
	defer span.End()

	existing := []kgo.RecordHeader{{Key: "x-custom", Value: []byte("v1")}}
	out := tracing.InjectKafka(ctx, existing)

	var found bool
	for _, h := range out {
		if h.Key == "x-custom" && string(h.Value) == "v1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pre-existing header preserved, got %+v", out)
	}
	if len(out) <= len(existing) {
		t.Fatalf("expected new headers appended; before=%d after=%d", len(existing), len(out))
	}
}

func TestExtractKafka_NoHeadersYieldsNoRemoteSpan(t *testing.T) {
	setupRecorder(t)

	ctx := tracing.ExtractKafka(context.Background(), nil)
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		t.Fatalf("expected invalid span context for empty headers, got %+v", sc)
	}
}

func TestInit_EmptyEndpointReturnsSafeNoopShutdown(t *testing.T) {
	// Don't mutate the global provider when endpoint is empty.
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	shutdown := tracing.Init("svc", "", logger.NewNoop())
	if shutdown == nil {
		t.Fatal("Init: expected non-nil shutdown func")
	}
	// Calling shutdown twice must be safe.
	shutdown()
	shutdown()

	// Provider must be unchanged.
	if otel.GetTracerProvider() != prev {
		t.Fatal("Init with empty endpoint should not mutate global provider")
	}
}

func TestInit_NonEmptyEndpointInstallsProviderAndPropagator(t *testing.T) {
	// Capture and restore globals; the OTLP exporter is lazy, so no real network call happens.
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})

	shutdown := tracing.Init("svc", "http://127.0.0.1:1/v1/traces", logger.NewNoop())
	t.Cleanup(shutdown)

	if otel.GetTracerProvider() == prevProvider {
		t.Fatal("Init: expected global TracerProvider to be replaced")
	}

	// The composite propagator must carry W3C traceparent on Inject.
	ctx, span := tracing.Start(context.Background(), "init-test")
	defer span.End()
	headers := tracing.InjectKafka(ctx, nil)
	var foundTraceparent bool
	for _, h := range headers {
		if h.Key == "traceparent" {
			foundTraceparent = true
		}
	}
	if !foundTraceparent {
		t.Fatalf("expected traceparent header after Init, got %+v", headers)
	}
}

// captureLogger records the last error-key field passed to Warn so we can
// verify the once-only export-failure handler.
type captureLogger struct {
	logger.Logger
	warnCalls int
	lastErr   error
}

func (c *captureLogger) Warn(_ string, fields ...logger.Field) {
	c.warnCalls++
	for _, f := range fields {
		if f.Key == logger.FieldError {
			if e, ok := f.Value.(error); ok {
				c.lastErr = e
			}
		}
	}
}
func (c *captureLogger) Debug(string, ...logger.Field)             {}
func (c *captureLogger) Info(string, ...logger.Field)              {}
func (c *captureLogger) Error(string, error, ...logger.Field)      {}
func (c *captureLogger) With(fields ...logger.Field) logger.Logger { return c }

func TestInit_ErrorHandlerLogsExportFailureOnce(t *testing.T) {
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	prevHandler := otel.GetErrorHandler()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
		otel.SetErrorHandler(prevHandler)
	})

	log := &captureLogger{}
	shutdown := tracing.Init("svc", "http://127.0.0.1:1/v1/traces", log)
	t.Cleanup(shutdown)

	// Trigger the global error handler twice — sync.Once should suppress the second log.
	otel.Handle(errors.New("first export failure"))
	otel.Handle(errors.New("second export failure"))

	if log.warnCalls != 1 {
		t.Fatalf("expected exactly one Warn call (once-only handler), got %d", log.warnCalls)
	}
	if log.lastErr == nil || log.lastErr.Error() != "first export failure" {
		t.Fatalf("expected first error captured, got %v", log.lastErr)
	}
}

// Ensure HTTP transport in otlptracehttp builds against a real-looking endpoint URL.
// This indirectly exercises the resource.Merge / NewTracerProvider path during Init.
func TestInit_AcceptsHTTPTestServerEndpoint(t *testing.T) {
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	shutdown := tracing.Init("svc", srv.URL+"/v1/traces", logger.NewNoop())
	t.Cleanup(shutdown)
}
