package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// zerologLogger wraps zerolog.Logger and implements the Logger interface.
type zerologLogger struct {
	l zerolog.Logger
}

// Init creates the process-wide logger for a service binary.
// Call once in main() before any other code runs.
//
// Reads env vars:
//
//	LOG_LEVEL: debug | info | warn | error  (default: info)
//	LOG_FORMAT: json | pretty               (default: json)
//
// The logger automatically includes:
//   - "service" field (from the service argument)
//   - "caller" field (file:line of the actual call site)
//   - "time" field (RFC3339)
func Init(service string) Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	return newWithWriter(resolveWriter(), service)
}

// NewWithWriter creates a logger that writes to w.
// Exported for use in tests only — production code uses Init().
func NewWithWriter(w io.Writer, service string) Logger {
	return newWithWriter(w, service)
}

func newWithWriter(w io.Writer, service string) Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	// CallerWithSkipFrameCount(3): our wrapper adds one extra frame over zerolog's
	// default (2), so we need 3 to point at the actual call site.
	l := zerolog.New(w).
		Level(level).
		With().
		Timestamp().
		Str(FieldService, service).
		CallerWithSkipFrameCount(3).
		Logger()

	return zerologLogger{l: l}
}

func resolveWriter() io.Writer {
	if os.Getenv("LOG_FORMAT") == "pretty" {
		return zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.TimeOnly}
	}
	return os.Stdout
}

func parseLevel(s string) zerolog.Level {
	switch s {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func (z zerologLogger) Debug(msg string, fields ...Field) {
	applyFields(z.l.Debug(), fields).Msg(msg)
}

func (z zerologLogger) Info(msg string, fields ...Field) {
	applyFields(z.l.Info(), fields).Msg(msg)
}

func (z zerologLogger) Warn(msg string, fields ...Field) {
	applyFields(z.l.Warn(), fields).Msg(msg)
}

func (z zerologLogger) Error(msg string, err error, fields ...Field) {
	applyFields(z.l.Error().Err(err), fields).Msg(msg)
}

func (z zerologLogger) With(fields ...Field) Logger {
	return zerologLogger{l: applyContext(z.l.With(), fields).Logger()}
}

// applyFields attaches the given fields to a zerolog Event before Msg() is called.
func applyFields(e *zerolog.Event, fields []Field) *zerolog.Event {
	for _, f := range fields {
		switch v := f.Value.(type) {
		case string:
			e = e.Str(f.Key, v)
		case int:
			e = e.Int(f.Key, v)
		case error:
			e = e.AnErr(f.Key, v)
		case time.Duration:
			e = e.Dur(f.Key, v)
		case bool:
			e = e.Bool(f.Key, v)
		}
	}
	return e
}

// applyContext attaches the given fields to a zerolog Context (used by With()).
func applyContext(ctx zerolog.Context, fields []Field) zerolog.Context {
	for _, f := range fields {
		switch v := f.Value.(type) {
		case string:
			ctx = ctx.Str(f.Key, v)
		case int:
			ctx = ctx.Int(f.Key, v)
		case error:
			ctx = ctx.AnErr(f.Key, v)
		case time.Duration:
			ctx = ctx.Dur(f.Key, v)
		case bool:
			ctx = ctx.Bool(f.Key, v)
		}
	}
	return ctx
}
