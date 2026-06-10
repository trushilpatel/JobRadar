package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ── Field constructors ────────────────────────────────────────────────────────

func TestStr(t *testing.T) {
	f := Str("key", "val")
	if f.Key != "key" || f.Value != "val" {
		t.Errorf("Str field = %+v, want key=key val=val", f)
	}
}

func TestInt(t *testing.T) {
	f := Int("count", 42)
	if f.Key != "count" || f.Value != 42 {
		t.Errorf("Int field = %+v, want key=count val=42", f)
	}
}

func TestDur(t *testing.T) {
	d := 5 * time.Second
	f := Dur("elapsed", d)
	if f.Key != "elapsed" || f.Value != d {
		t.Errorf("Dur field = %+v, want key=elapsed val=%v", f, d)
	}
}

func TestBool(t *testing.T) {
	f := Bool("ok", true)
	if f.Key != "ok" || f.Value != true {
		t.Errorf("Bool field = %+v, want key=ok val=true", f)
	}
}

func TestErr(t *testing.T) {
	e := errors.New("oops")
	f := Err(e)
	if f.Key != FieldError || f.Value != e {
		t.Errorf("Err field = %+v, want key=%q", f, FieldError)
	}
}

// ── parseLevel ────────────────────────────────────────────────────────────────

func TestParseLevel_DebugLevelAllowsDebugMessages(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Debug("visible debug")
	if !strings.Contains(buf.String(), "visible debug") {
		t.Error("expected debug message to appear at debug level")
	}
}

func TestParseLevel_WarnLevelSuppressesInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Info("should be suppressed")
	if strings.Contains(buf.String(), "should be suppressed") {
		t.Error("expected info message to be suppressed at warn level")
	}
}

func TestParseLevel_ErrorLevelSuppressesWarn(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Warn("should be suppressed")
	if strings.Contains(buf.String(), "should be suppressed") {
		t.Error("expected warn message to be suppressed at error level")
	}
}

func TestParseLevel_DefaultIsInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Info("info visible")
	if !strings.Contains(buf.String(), "info visible") {
		t.Error("expected info message to appear at default (info) level")
	}
}

// ── resolveWriter ─────────────────────────────────────────────────────────────

func TestResolveWriter_DefaultReturnsNonNil(t *testing.T) {
	t.Setenv("LOG_FORMAT", "")
	w := resolveWriter()
	if w == nil {
		t.Fatal("resolveWriter should not return nil for default format")
	}
}

func TestResolveWriter_PrettyFormatReturnsNonNil(t *testing.T) {
	t.Setenv("LOG_FORMAT", "pretty")
	w := resolveWriter()
	if w == nil {
		t.Fatal("resolveWriter should not return nil for pretty format")
	}
	// Exercise the writer path — must not panic.
	log := newWithWriter(w, "svc")
	log.Info("pretty format smoke test")
}

// ── Constructors ──────────────────────────────────────────────────────────────

func TestInit_ReturnsNonNil(t *testing.T) {
	// Init writes to stdout — just verify it returns a usable logger.
	log := Init("test-svc")
	if log == nil {
		t.Fatal("Init should return a non-nil Logger")
	}
}

func TestNewNoop_DiscardsSilently(t *testing.T) {
	log := NewNoop()
	// None of these must panic or write anywhere.
	log.Debug("d")
	log.Info("i")
	log.Warn("w")
	log.Error("e", errors.New("err"))
}

func TestNewWithWriter_WritesToBuffer(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, "svc")
	log.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected 'hello' in output, got: %q", buf.String())
	}
}

// ── Log methods ───────────────────────────────────────────────────────────────

func TestDebug_WritesMessage(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Debug("dbg-msg")
	if !strings.Contains(buf.String(), "dbg-msg") {
		t.Errorf("expected 'dbg-msg' in debug output, got: %q", buf.String())
	}
}

func TestInfo_WritesMessage(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Info("info-msg")
	if !strings.Contains(buf.String(), "info-msg") {
		t.Errorf("expected 'info-msg' in info output, got: %q", buf.String())
	}
}

func TestWarn_WritesMessage(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Warn("warn-msg")
	if !strings.Contains(buf.String(), "warn-msg") {
		t.Errorf("expected 'warn-msg' in warn output, got: %q", buf.String())
	}
}

func TestError_WritesMessageAndError(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	log.Error("err-msg", errors.New("underlying cause"))
	out := buf.String()
	if !strings.Contains(out, "err-msg") {
		t.Errorf("expected 'err-msg' in error output, got: %q", out)
	}
	if !strings.Contains(out, "underlying cause") {
		t.Errorf("expected 'underlying cause' in error output, got: %q", out)
	}
}

// ── applyFields — all type branches via Info ───────────────────────────────────

func TestApplyFields_AllTypes(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	fieldErr := errors.New("field-error")

	log.Info("all-fields",
		Str("s", "hello"),
		Int("n", 7),
		Err(fieldErr),
		Dur("d", 100*time.Millisecond),
		Bool("b", true),
	)

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON output: %v — raw: %q", err, buf.String())
	}
	if got := out["s"]; got != "hello" {
		t.Errorf("Str field: got %v, want %q", got, "hello")
	}
	if n, _ := out["n"].(float64); n != 7 {
		t.Errorf("Int field: got %v, want 7", out["n"])
	}
	if out[FieldError] == nil {
		t.Errorf("Err field: expected non-nil %q key", FieldError)
	}
	if out["d"] == nil {
		t.Errorf("Dur field: expected non-nil 'd' key")
	}
	if b, _ := out["b"].(bool); !b {
		t.Errorf("Bool field: got %v, want true", out["b"])
	}
}

// ── applyContext — all type branches via With ─────────────────────────────────

func TestWith_BakesAllFieldTypesIntoEvents(t *testing.T) {
	var buf bytes.Buffer
	base := newWithWriter(&buf, "svc")
	child := base.With(
		Str("adapter", "test-adapter"),
		Int("count", 3),
		Bool("flag", true),
		Dur("elapsed", 50*time.Millisecond),
		Err(errors.New("baked-err")),
	)
	child.Info("event")

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v — raw: %q", err, buf.String())
	}
	if out["adapter"] != "test-adapter" {
		t.Errorf("expected baked adapter='test-adapter', got %v", out["adapter"])
	}
	if n, _ := out["count"].(float64); n != 3 {
		t.Errorf("expected baked count=3, got %v", out["count"])
	}
	if b, _ := out["flag"].(bool); !b {
		t.Errorf("expected baked flag=true, got %v", out["flag"])
	}
}

// ── WithContext ───────────────────────────────────────────────────────────────

func TestWithContext_InvalidSpanReturnsUnchangedLogger(t *testing.T) {
	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	// Background context carries no span — IsValid() returns false.
	child := log.WithContext(context.Background())
	child.Info("no-span")
	out := buf.String()
	if strings.Contains(out, FieldTraceID) {
		t.Errorf("expected no %q field for invalid span, got: %q", FieldTraceID, out)
	}
}

func TestWithContext_ValidSpanAddsTraceAndSpanID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	var buf bytes.Buffer
	log := newWithWriter(&buf, "svc")
	child := log.WithContext(ctx)
	child.Info("with-span")

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v — raw: %q", err, buf.String())
	}
	if _, ok := out[FieldTraceID]; !ok {
		t.Errorf("expected %q field in output with valid span", FieldTraceID)
	}
	if _, ok := out[FieldSpanID]; !ok {
		t.Errorf("expected %q field in output with valid span", FieldSpanID)
	}
}
