package logger

import "context"

// Logger is the logging interface used across all workers and adapters.
// Identical to the backend logger interface with one addition: WithContext
// for automatic trace_id + span_id injection from an OTel span.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, err error, fields ...Field)

	// With returns a child logger with the given fields baked into every future event.
	With(fields ...Field) Logger

	// WithContext returns a child logger with trace_id + span_id baked in
	// from the active OTel span in ctx. Call this once at the start of each
	// unit of work (per company scan, per job fetch) and use the child logger
	// for all subsequent calls within that scope.
	WithContext(ctx context.Context) Logger
}
