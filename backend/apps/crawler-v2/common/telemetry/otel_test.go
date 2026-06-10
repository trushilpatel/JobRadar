package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

func TestInit_EmptyEndpointReturnsNoop(t *testing.T) {
	shutdown := Init("svc", "", logger.NewNoop())
	if shutdown == nil {
		t.Fatal("Init with empty endpoint should return non-nil shutdown func")
	}
	shutdown() // must not panic
}

func TestInit_InvalidEndpointDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Init panicked on invalid endpoint: %v", r)
		}
	}()
	shutdown := Init("svc", "://bad-url", logger.NewNoop())
	if shutdown == nil {
		t.Fatal("Init with bad endpoint should return non-nil shutdown func")
	}
	shutdown()
}

func TestInit_ValidEndpointReturnsShutdown(t *testing.T) {
	// A reachable-looking but unreachable endpoint should still return a
	// shutdown func (export errors are non-fatal, handled by the error handler).
	shutdown := Init("svc", "http://127.0.0.1:19999", logger.NewNoop())
	if shutdown == nil {
		t.Fatal("Init should always return a non-nil shutdown func")
	}
	shutdown()
}

func TestStart_ReturnsContextAndSpan(t *testing.T) {
	ctx, span := Start(context.Background(), "test-op")
	defer span.End()
	if ctx == nil {
		t.Fatal("Start should return non-nil context")
	}
	if span == nil {
		t.Fatal("Start should return non-nil span")
	}
}

func TestEnd_NilErrPtrDoesNotPanic(t *testing.T) {
	_, span := Start(context.Background(), "op")
	End(span, nil) // must not panic
}

func TestEnd_NilErrorDoesNotSetStatus(t *testing.T) {
	_, span := Start(context.Background(), "op")
	var err error
	End(span, &err) // err is nil — must not set error status
}

func TestEnd_RecordsNonNilError(t *testing.T) {
	_, span := Start(context.Background(), "op")
	err := errors.New("something failed")
	End(span, &err) // must not panic; records error on span
}

func TestInit_ErrorHandlerLogsOnce(t *testing.T) {
	// Init registers a custom error handler. Triggering it via otel.Handle
	// covers the exportWarnOnce.Do + log.Warn path inside the closure.
	shutdown := Init("svc", "http://127.0.0.1:19999", logger.NewNoop())
	defer shutdown()
	otel.Handle(errors.New("simulated export failure"))
}

func TestInit_ExporterError_ReturnsNoop(t *testing.T) {
	orig := newExporter
	defer func() { newExporter = orig }()
	newExporter = func(_ context.Context, _ string) (sdktrace.SpanExporter, error) {
		return nil, errors.New("injected exporter error")
	}
	shutdown := Init("svc", "http://any-endpoint", logger.NewNoop())
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown func")
	}
	shutdown() // must not panic
}
