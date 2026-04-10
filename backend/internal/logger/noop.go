package logger

// noopLogger discards all log output. Use in unit tests to suppress noise.
type noopLogger struct{}

// NewNoop returns a Logger that silently discards all events.
func NewNoop() Logger { return noopLogger{} }

func (noopLogger) Debug(msg string, fields ...Field)            {}
func (noopLogger) Info(msg string, fields ...Field)             {}
func (noopLogger) Warn(msg string, fields ...Field)             {}
func (noopLogger) Error(msg string, err error, fields ...Field) {}
func (n noopLogger) With(fields ...Field) Logger                { return n }
