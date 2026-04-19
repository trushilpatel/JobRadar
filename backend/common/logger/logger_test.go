package logger_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// Compile-time checks: both implementations must satisfy the Logger interface.
var _ logger.Logger = logger.NewNoop()

func TestNoopLogger_DoesNotPanic(t *testing.T) {
	log := logger.NewNoop()
	log.Debug("debug message", logger.Str("key", "val"))
	log.Info("info message", logger.Int("count", 5))
	log.Warn("warn message", logger.Dur("elapsed", time.Second))
	log.Error("error message", errors.New("some error"), logger.Bool("flag", true))
	child := log.With(logger.Str(logger.FieldJobID, "job-123"))
	child.Info("child message")
}

func newTestLogger(buf *bytes.Buffer) logger.Logger {
	return logger.NewWithWriter(buf, "test-svc")
}

func TestZerologLogger_InfoOutputsJSON(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.Info("scan complete", logger.Str(logger.FieldCompany, "stripe"), logger.Int(logger.FieldFetched, 10))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
	if entry["message"] != "scan complete" {
		t.Errorf("expected message='scan complete', got %v", entry["message"])
	}
	if entry[logger.FieldService] != "test-svc" {
		t.Errorf("expected service=test-svc, got %v", entry[logger.FieldService])
	}
	if entry[logger.FieldCompany] != "stripe" {
		t.Errorf("expected company_id=stripe, got %v", entry[logger.FieldCompany])
	}
	if entry[logger.FieldFetched] != float64(10) {
		t.Errorf("expected fetched=10, got %v", entry[logger.FieldFetched])
	}
}

func TestZerologLogger_ErrorIncludesErrField(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.Error("publish failed", errors.New("connection refused"))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if entry["level"] != "error" {
		t.Errorf("expected level=error, got %v", entry["level"])
	}
	if entry["error"] != "connection refused" {
		t.Errorf("expected error='connection refused', got %v", entry["error"])
	}
}

func TestZerologLogger_WithBakesFields(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	child := log.With(logger.Str(logger.FieldJobID, "job-abc"))

	child.Info("enriched")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if entry[logger.FieldJobID] != "job-abc" {
		t.Errorf("expected job_id=job-abc on child logger, got %v", entry[logger.FieldJobID])
	}
}
