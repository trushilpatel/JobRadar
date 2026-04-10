package logger

// Logger is the logging interface used across all workers and the API.
// Workers receive this via constructor injection — never a concrete type.
//
// Switching logging backends (zerolog → slog → zap) requires only replacing
// the implementation in zerolog.go. Call sites are unaffected.
type Logger interface {
	// Debug logs at DEBUG level. Suppressed when LOG_LEVEL=info or higher.
	// Use for per-item events, routine steps, sleep intervals.
	Debug(msg string, fields ...Field)

	// Info logs meaningful milestones: worker started, cycle complete, job saved.
	Info(msg string, fields ...Field)

	// Warn logs recoverable issues: skipped items, flush timeouts, rate limit waits.
	Warn(msg string, fields ...Field)

	// Error logs actual failures: DB write failed, LLM call failed, publish failed.
	// err is logged under the "error" key automatically.
	Error(msg string, err error, fields ...Field)

	// With returns a child logger with the given fields baked into every future event.
	// Use this at the start of a job-processing loop to attach job_id and company_id.
	With(fields ...Field) Logger
}
