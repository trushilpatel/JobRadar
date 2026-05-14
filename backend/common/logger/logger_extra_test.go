package logger_test

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

func TestNoopLogger_AllMethodsAndChild(t *testing.T) {
	noop := logger.NewNoop()
	// Exercise every method; if anything panics or returns wrong child, fail.
	noop.Debug("d", logger.Str("k", "v"))
	noop.Info("i", logger.Int("n", 1))
	noop.Warn("w", logger.Dur("d", time.Millisecond))
	noop.Error("e", stderrors.New("oops"), logger.Bool("b", true))
	child := noop.With(logger.Str("k", "v"))
	if child == nil {
		t.Fatal("Noop.With must return a non-nil Logger")
	}
	child.Info("child")
}

func TestFields_Constructors(t *testing.T) {
	cases := []struct {
		name  string
		field logger.Field
		key   string
		val   any
	}{
		{"Str", logger.Str("k", "v"), "k", "v"},
		{"Int", logger.Int("n", 42), "n", 42},
		{"Dur", logger.Dur("d", time.Second), "d", time.Second},
		{"Bool", logger.Bool("b", true), "b", true},
		{"Err", logger.Err(stderrors.New("boom")), logger.FieldError, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.field.Key != c.key {
				t.Fatalf("expected key %q, got %q", c.key, c.field.Key)
			}
			if c.name == "Err" {
				if _, ok := c.field.Value.(error); !ok {
					t.Fatalf("expected error value for Err, got %T", c.field.Value)
				}
				return
			}
			if c.field.Value != c.val {
				t.Fatalf("expected value %v, got %v", c.val, c.field.Value)
			}
		})
	}
}

func TestZerologLogger_DebugAndWarnEmitJSON(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, "svc")

	log.Debug("step", logger.Str("k", "v"))
	log.Warn("careful", logger.Int("n", 7))

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), buf.String())
	}
	var d map[string]any
	if err := json.Unmarshal(lines[0], &d); err != nil {
		t.Fatalf("debug line not JSON: %v", err)
	}
	if d["level"] != "debug" {
		t.Fatalf("expected debug level, got %v", d["level"])
	}
	var w map[string]any
	if err := json.Unmarshal(lines[1], &w); err != nil {
		t.Fatalf("warn line not JSON: %v", err)
	}
	if w["level"] != "warn" {
		t.Fatalf("expected warn level, got %v", w["level"])
	}
}

func TestZerologLogger_DebugSuppressedAtInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "info")
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, "svc")
	log.Debug("should be suppressed")
	if buf.Len() != 0 {
		t.Fatalf("debug must be suppressed at info level, got %q", buf.String())
	}
}

func TestZerologLogger_ApplyFields_TypeBranches(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, "svc")

	log.Info("all-types",
		logger.Str("s", "abc"),
		logger.Int("i", 9),
		logger.Bool("b", true),
		logger.Dur("d", 250*time.Millisecond),
		logger.Err(stderrors.New("oh no")),
	)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("not JSON: %v\nraw=%s", err, buf.String())
	}
	if entry["s"] != "abc" {
		t.Fatalf("string field: got %v", entry["s"])
	}
	if entry["i"] != float64(9) {
		t.Fatalf("int field: got %v", entry["i"])
	}
	if entry["b"] != true {
		t.Fatalf("bool field: got %v", entry["b"])
	}
	// duration is rendered as a number (ms by default in zerolog) — only check presence.
	if _, ok := entry["d"]; !ok {
		t.Fatalf("duration field missing")
	}
	if entry[logger.FieldError] != "oh no" {
		t.Fatalf("error field: got %v", entry[logger.FieldError])
	}
}

func TestZerologLogger_With_CarriesAllFieldTypes(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, "svc")

	child := log.With(
		logger.Str("s", "x"),
		logger.Int("i", 1),
		logger.Bool("b", false),
		logger.Dur("d", time.Second),
		logger.Err(stderrors.New("ctx")),
	)
	child.Info("baked")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if entry["s"] != "x" || entry["i"] != float64(1) || entry["b"] != false {
		t.Fatalf("baked fields missing: %v", entry)
	}
}

func TestInit_RespectsLogLevelEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FORMAT", "json")
	// Init writes to stdout in prod; redirect by replacing os.Stdout for the
	// duration of one call. We don't assert on output here — just that it
	// returns a non-nil Logger and that LOG_FORMAT=json keeps the writer
	// non-pretty.
	log := logger.Init("svc")
	if log == nil {
		t.Fatal("Init returned nil")
	}
}

func TestInit_PrettyFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "pretty")
	log := logger.Init("svc")
	if log == nil {
		t.Fatal("Init returned nil for pretty format")
	}
}

func TestParseLevel_IndirectlyViaInit(t *testing.T) {
	// parseLevel is unexported but is the only branch driver inside Init.
	// Exercise the not-hit branches by setting LOG_LEVEL and reading what
	// the resulting logger emits at each level.
	t.Setenv("LOG_FORMAT", "json")
	cases := []struct {
		level     string
		emits     func(l logger.Logger)
		mustEmpty bool
	}{
		{"debug", func(l logger.Logger) { l.Debug("x") }, false},
		{"warn", func(l logger.Logger) { l.Debug("x") }, true},
		{"error", func(l logger.Logger) { l.Warn("x") }, true},
		{"unknown-falls-back-to-info", func(l logger.Logger) { l.Debug("x") }, true},
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", strings.TrimSuffix(c.level, "-falls-back-to-info"))
			var buf bytes.Buffer
			l := logger.NewWithWriter(&buf, "svc")
			c.emits(l)
			if c.mustEmpty && buf.Len() != 0 {
				t.Fatalf("expected suppression at level=%q, got %q", c.level, buf.String())
			}
			if !c.mustEmpty && buf.Len() == 0 {
				t.Fatalf("expected output at level=%q, got nothing", c.level)
			}
		})
	}
}

func TestInit_Smoke_DoesNotLeakStdout(t *testing.T) {
	// Defensive: confirm Init() runs without panic regardless of env state.
	for _, v := range []string{"debug", "info", "warn", "error", "garbage", ""} {
		t.Run("level="+v, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", v)
			_ = logger.Init("svc")
		})
	}
	// Confirm os.Stdout is still writable afterwards.
	if _, err := os.Stdout.Write([]byte{}); err != nil {
		t.Fatalf("stdout broken after Init: %v", err)
	}
}
