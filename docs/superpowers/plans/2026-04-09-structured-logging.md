# Structured Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all stdlib `log.Printf` calls across 5 workers and the API with zerolog-based structured logging, providing log levels, JSON output, auto caller info, and pipeline correlation fields.

**Architecture:** A thin `internal/logger/` package defines a `Logger` interface backed by zerolog. Workers receive the interface via constructor injection. All log lines carry `service`, `caller`, and relevant structured fields automatically.

**Tech Stack:** `github.com/rs/zerolog` (single new dependency), Go stdlib `os`, `io`, `time`

---

## File Map

| Action | File | Responsibility |
|---|---|---|
| Create | `backend/internal/logger/fields.go` | Field type, helper constructors, field name constants |
| Create | `backend/internal/logger/logger.go` | Logger interface |
| Create | `backend/internal/logger/noop.go` | No-op implementation for tests |
| Create | `backend/internal/logger/zerolog.go` | zerolog implementation + Init() |
| Create | `backend/internal/logger/logger_test.go` | Tests for all logger implementations |
| Modify | `backend/internal/kafka/consumer.go` | Add logger field + inject via NewConsumer |
| Modify | `backend/internal/llm/ratelimit.go` | Add logger field to rateLimitedScorer |
| Modify | `backend/internal/llm/factory.go` | Add Log field to ScorerOptions, thread through |
| Modify | `backend/config/config.go` | Replace log.Println with fmt.Fprintf (pre-logger) |
| Modify | `backend/internal/worker/job_listing_crawler/engine.go` | Add logger, replace all log.Printf |
| Modify | `backend/internal/worker/job_listing_relevance_filter/worker.go` | Add logger, replace all log.Printf |
| Modify | `backend/internal/worker/job_post_analyzer/worker.go` | Add logger, replace all log.Printf |
| Modify | `backend/internal/api/middleware/logger.go` | Factory pattern accepting logger.Logger |
| Modify | `backend/internal/api/router.go` | Accept logger.Logger, pass to middleware |
| Modify | `backend/cmd/company_scan_scheduler/main.go` | Init logger, wire through |
| Modify | `backend/cmd/job_listing_crawler/main.go` | Init logger, wire through |
| Modify | `backend/cmd/job_listing_relevance_filter/main.go` | Init logger, wire through |
| Modify | `backend/cmd/job_post_analyzer/main.go` | Init logger, wire through |
| Modify | `backend/cmd/api/main.go` | Init logger, wire through |
| Modify | `backend/docker-compose.yml` | Add LOG_LEVEL and LOG_FORMAT env vars |

---

## Task 1: Add zerolog dependency

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add zerolog**

```bash
cd backend && go get github.com/rs/zerolog@latest
```

Expected: `go.mod` now contains `github.com/rs/zerolog` in `require` block.

- [ ] **Step 2: Verify it downloaded**

```bash
cd backend && go mod tidy
```

Expected: no errors, `go.sum` updated.

---

## Task 2: Create `internal/logger/fields.go`

**Files:**
- Create: `backend/internal/logger/fields.go`

- [ ] **Step 1: Write fields.go**

```go
package logger

import "time"

// Field is a typed key-value pair attached to a log event.
// Use the constructor functions below — never construct Field directly.
type Field struct {
	Key   string
	Value any
}

// Standardized field key constants. Always use these — never raw strings.
// Consistent keys are required for Loki queries to work across services.
const (
	FieldService  = "service"
	FieldCompany  = "company_id"
	FieldJobID    = "job_id"
	FieldDuration = "duration"
	FieldFetched  = "fetched"
	FieldNew      = "new"
	FieldError    = "error"
	FieldBatch    = "batch"
	FieldRelevant = "relevant"
	FieldMethod   = "method"
	FieldPath     = "path"
	FieldStatus   = "status"
)

// Str creates a string field.
func Str(key, val string) Field { return Field{Key: key, Value: val} }

// Int creates an integer field.
func Int(key string, val int) Field { return Field{Key: key, Value: val} }

// Dur creates a duration field.
func Dur(key string, d time.Duration) Field { return Field{Key: key, Value: d} }

// Bool creates a boolean field.
func Bool(key string, val bool) Field { return Field{Key: key, Value: val} }

// Err creates an error field using FieldError as the key.
// Use this to attach an error at a non-Error log level (e.g. Warn).
// At Error level, pass err directly to Logger.Error() instead.
func Err(err error) Field { return Field{Key: FieldError, Value: err} }
```

---

## Task 3: Create `internal/logger/logger.go`

**Files:**
- Create: `backend/internal/logger/logger.go`

- [ ] **Step 1: Write logger.go**

```go
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
```

---

## Task 4: Create `internal/logger/noop.go`

**Files:**
- Create: `backend/internal/logger/noop.go`
- Create: `backend/internal/logger/logger_test.go` (partial — noop tests)

- [ ] **Step 1: Write the failing test first**

Create `backend/internal/logger/logger_test.go`:

```go
package logger_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/internal/logger"
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
```

- [ ] **Step 2: Run to verify it fails (noop doesn't exist yet)**

```bash
cd backend && go test ./internal/logger/...
```

Expected: compilation error — `logger.NewNoop undefined`

- [ ] **Step 3: Write noop.go**

```go
package logger

// noopLogger discards all log output. Use in unit tests to suppress noise.
type noopLogger struct{}

// NewNoop returns a Logger that silently discards all events.
func NewNoop() Logger { return noopLogger{} }

func (noopLogger) Debug(msg string, fields ...Field) {}
func (noopLogger) Info(msg string, fields ...Field)  {}
func (noopLogger) Warn(msg string, fields ...Field)  {}
func (noopLogger) Error(msg string, err error, fields ...Field) {}
func (n noopLogger) With(fields ...Field) Logger { return n }
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/logger/...
```

Expected: PASS (noop tests pass)

---

## Task 5: Create `internal/logger/zerolog.go`

**Files:**
- Create: `backend/internal/logger/zerolog.go`
- Modify: `backend/internal/logger/logger_test.go` (add zerolog tests)

- [ ] **Step 1: Add zerolog tests to logger_test.go**

Append to `backend/internal/logger/logger_test.go`:

```go
// newTestLogger creates a zerolog logger writing to buf with no timestamps
// (timestamps would make assertions flaky). Used only in tests.
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
```

- [ ] **Step 2: Run to verify tests fail**

```bash
cd backend && go test ./internal/logger/...
```

Expected: compilation error — `logger.NewWithWriter undefined`

- [ ] **Step 3: Write zerolog.go**

```go
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
//   LOG_LEVEL: debug | info | warn | error  (default: info)
//   LOG_FORMAT: json | pretty               (default: json)
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
```

- [ ] **Step 4: Run all logger tests**

```bash
cd backend && go test ./internal/logger/... -v
```

Expected: all tests PASS

---

## Task 6: Update `internal/kafka/consumer.go`

**Files:**
- Modify: `backend/internal/kafka/consumer.go`

- [ ] **Step 1: Add logger field to Consumer and update NewConsumer**

Replace the `Consumer` struct and `NewConsumer` function:

```go
// Consumer struct — add log field
type Consumer struct {
	client *kgo.Client
	log    logger.Logger
}

// NewConsumer — add log parameter (second argument, after cfg)
func NewConsumer(cfg *config.Config, log logger.Logger, groupID string, topics ...string) (*Consumer, error) {
	opts := []kgo.Opt{
		// ... existing opts unchanged ...
	}
	// ... existing SASL block unchanged ...

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	return &Consumer{client: client, log: log}, nil
}
```

- [ ] **Step 2: Update the Poll method's log call**

Find the `log.Printf("[kafka] fetch error (retrying): %v", err)` line in the Poll method and replace with:

```go
c.log.Warn("fetch error, retrying", logger.Err(err))
```

- [ ] **Step 3: Update imports**

Replace `"log"` import with:

```go
"github.com/trushilpatel/jobradar/internal/logger"
```

- [ ] **Step 4: Verify the package compiles**

```bash
cd backend && go build ./internal/kafka/...
```

Expected: compilation error in cmd/ binaries (they call `NewConsumer` with old signature). That's expected — fixed in Task 13.

---

## Task 7: Update `internal/llm/ratelimit.go` and `factory.go`

**Files:**
- Modify: `backend/internal/llm/ratelimit.go`
- Modify: `backend/internal/llm/factory.go`

- [ ] **Step 1: Add logger to rateLimitedScorer struct**

In `ratelimit.go`, add `log logger.Logger` field to `rateLimitedScorer`:

```go
type rateLimitedScorer struct {
	inner Scorer
	log   logger.Logger

	rpm *rate.Limiter
	tpm *rate.Limiter

	mu       sync.Mutex
	dayCount int
	dayMax   int
	dayReset time.Time
}
```

- [ ] **Step 2: Update newRateLimitedScorer to accept logger**

```go
func newRateLimitedScorer(inner Scorer, limits RateLimits, log logger.Logger) Scorer {
	s := &rateLimitedScorer{
		inner: inner,
		log:   log,
		// ... existing field assignments unchanged ...
	}
	// ... rest of constructor unchanged ...
	return s
}
```

- [ ] **Step 3: Replace the log.Printf in the daily limit block**

Find `log.Printf("[llm] daily request limit (%d RPD) reached — sleeping until %s", ...)` and replace with:

```go
s.log.Warn("daily request limit reached, sleeping",
	logger.Int("rpd_limit", s.dayMax),
	logger.Str("reset_at", s.dayReset.Format(time.RFC3339)),
)
```

- [ ] **Step 4: Update imports in ratelimit.go**

Replace `"log"` import with:

```go
"github.com/trushilpatel/jobradar/internal/logger"
```

- [ ] **Step 5: Add Log field to ScorerOptions in factory.go**

```go
type ScorerOptions struct {
	Provider    string
	APIKey      string
	Model       string
	TargetRoles []string
	YearsExp    int
	Limits      RateLimits
	Log         logger.Logger // used by the rate limiter
}
```

- [ ] **Step 6: Thread Log through NewScorer in factory.go**

Update the rate limiter wrap call:

```go
if opts.Limits.RPM > 0 || opts.Limits.RPD > 0 || opts.Limits.TPMIn > 0 {
	return newRateLimitedScorer(inner, opts.Limits, opts.Log), nil
}
```

- [ ] **Step 7: Update imports in factory.go**

Add:

```go
"github.com/trushilpatel/jobradar/internal/logger"
```

- [ ] **Step 8: Verify llm package compiles**

```bash
cd backend && go build ./internal/llm/...
```

Expected: PASS (ScorerOptions.Log defaults to nil — nil logger causes panic at rate limit, fixed in Task 13 where we inject it)

---

## Task 8: Update `config/config.go`

**Files:**
- Modify: `backend/config/config.go`

- [ ] **Step 1: Replace log.Println with fmt.Fprintf**

Find:

```go
log.Println("no .env file found, falling back to environment variables")
```

Replace with:

```go
fmt.Fprintf(os.Stderr, "config: no .env file found, falling back to environment variables\n")
```

This runs before the logger is initialized, so stdlib log is inappropriate here.

- [ ] **Step 2: Update imports**

Remove `"log"` import. Add `"fmt"` and `"os"` if not already present.

- [ ] **Step 3: Verify**

```bash
cd backend && go build ./config/...
```

Expected: PASS

---

## Task 9: Update `internal/worker/job_listing_crawler/engine.go`

**Files:**
- Modify: `backend/internal/worker/job_listing_crawler/engine.go`

- [ ] **Step 1: Add logger to Engine struct and constructor**

```go
type Engine struct {
	companies    store.CompanyReader
	ledger       store.SeenLedgerStore
	scanLog      store.CompanyScanLogWriter
	publisher    kafka.Publisher
	companyDelay time.Duration
	log          logger.Logger
}

func NewEngine(
	companies store.CompanyReader,
	ledger store.SeenLedgerStore,
	scanLog store.CompanyScanLogWriter,
	publisher kafka.Publisher,
	companyDelay time.Duration,
	log logger.Logger,
) *Engine {
	return &Engine{
		companies:    companies,
		ledger:       ledger,
		scanLog:      scanLog,
		publisher:    publisher,
		companyDelay: companyDelay,
		log:          log,
	}
}
```

- [ ] **Step 2: Replace all log calls in RunOnce**

```go
func (e *Engine) RunOnce(ctx context.Context) {
	companies, err := e.companies.GetActiveCompanies()
	if err != nil {
		e.log.Error("failed to load companies", err)
		return
	}
	e.log.Info("starting scan cycle", logger.Int("active_companies", len(companies)))
	for _, company := range companies {
		e.scanCompany(ctx, company)
	}
	e.log.Info("scan cycle complete")
}
```

- [ ] **Step 3: Replace all log calls in RunForCompany**

```go
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	company, err := e.companies.GetCompanyByID(companyID)
	if err != nil {
		e.log.Error("company not found", err, logger.Str(logger.FieldCompany, companyID))
		return
	}
	if !company.Active {
		e.log.Warn("skipping inactive company",
			logger.Str(logger.FieldCompany, company.ID),
			logger.Str("name", company.Name),
		)
		return
	}
	e.log.Debug("triggered scan",
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)
	e.scanCompany(ctx, company)
}
```

- [ ] **Step 4: Replace all log calls in scanCompany**

```go
func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()
	companyLog := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	adp, err := ats_fetcher.Registry(company.AdapterType)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		companyLog.Error("no adapter found", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		companyLog.Error("fetch jobs failed", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	fetched := len(result.Jobs)
	newCount := 0

	for _, item := range result.Jobs {
		seen, err := e.ledger.HasSeen(item.ID)
		if err != nil {
			companyLog.Error("seen_ledger check failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}
		if seen {
			continue
		}

		if err := e.ledger.MarkSeen(item.ID, company.ID, model.SeenReasonFetch); err != nil {
			companyLog.Error("failed to mark seen", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		rawJob := toRawJob(item, company.ID, company.AdapterType)
		if err := e.publish(ctx, rawJob); err != nil {
			companyLog.Error("publish failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		newCount++
	}

	companyLog.Info("scan complete",
		logger.Int(logger.FieldFetched, fetched),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, time.Since(start).Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, fetched, newCount, time.Since(start), nil)

	if e.companyDelay > 0 {
		companyLog.Debug("sleeping before next company", logger.Dur("delay", e.companyDelay))
		select {
		case <-time.After(e.companyDelay):
		case <-ctx.Done():
		}
	}
}
```

- [ ] **Step 5: Replace log calls in recordResult**

```go
func (e *Engine) recordResult(ctx context.Context, companyID string, fetched, newCount int, duration time.Duration, errMsg *string) {
	status := model.ScanStatusOK
	if errMsg != nil {
		status = model.ScanStatusError
	}
	durationMs := int(duration.Milliseconds())
	if err := e.scanLog.InsertCompanyScanLog(companyID, fetched, newCount, durationMs, errMsg); err != nil {
		e.log.Error("failed to write scan log", err, logger.Str(logger.FieldCompany, companyID))
	}
	if err := e.scanLog.UpdateCompanyScanStatus(companyID, status); err != nil {
		e.log.Error("failed to update company status", err, logger.Str(logger.FieldCompany, companyID))
	}
}
```

- [ ] **Step 6: Update imports**

Replace `"log"` with `"github.com/trushilpatel/jobradar/internal/logger"`.

- [ ] **Step 7: Verify**

```bash
cd backend && go build ./internal/worker/job_listing_crawler/...
```

Expected: PASS (cmd/ will fail — fixed in Task 13)

---

## Task 10: Update `internal/worker/job_listing_relevance_filter/worker.go`

**Files:**
- Modify: `backend/internal/worker/job_listing_relevance_filter/worker.go`

- [ ] **Step 1: Add logger to Worker struct and constructor**

```go
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llm.Scorer
	ledger       store.SeenLedgerWriter
	batchSize    int
	flushTimeout time.Duration
	log          logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llm.Scorer,
	ledger store.SeenLedgerWriter,
	batchSize int,
	flushTimeout time.Duration,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		ledger:       ledger,
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		log:          log,
	}
}
```

- [ ] **Step 2: Replace all log calls in the Run method and helpers**

In the `Run` method, replace:

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] worker started (batchSize=%d flushTimeout=%s)", w.batchSize, w.flushTimeout)

// AFTER
w.log.Info("worker started",
	logger.Int("batch_size", w.batchSize),
	logger.Dur("flush_timeout", w.flushTimeout),
)
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] malformed message: %v — skipping", err)

// AFTER
w.log.Warn("malformed message, skipping", logger.Err(err))
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] flush timeout — flushing %d buffered jobs", len(batch))

// AFTER
w.log.Debug("flush timeout, flushing batch", logger.Int(logger.FieldBatch, len(batch)))
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] shutdown — flushing %d remaining jobs", len(batch))

// AFTER
w.log.Info("shutdown, flushing remaining jobs", logger.Int(logger.FieldBatch, len(batch)))
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] ScoreBatch failed: %v — marking batch of %d as error", err, len(batch))

// AFTER
w.log.Error("ScoreBatch failed, marking batch as error", err, logger.Int(logger.FieldBatch, len(batch)))
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] mark error batch failed: %v", merr)

// AFTER
w.log.Error("mark error batch failed", merr)
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] batch mark irrelevant failed: %v", err)

// AFTER
w.log.Error("mark irrelevant failed", err)
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] batch=%d relevant=%d irrelevant=%d", len(batch), len(relevantIDs), len(batch)-len(relevantIDs))

// AFTER
w.log.Info("batch scored",
	logger.Int(logger.FieldBatch, len(batch)),
	logger.Int(logger.FieldRelevant, len(relevantIDs)),
	logger.Int("irrelevant", len(batch)-len(relevantIDs)),
)
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] marshal failed for %s: %v", job.ID, err)

// AFTER
w.log.Error("marshal failed", err, logger.Str(logger.FieldJobID, job.ID))
```

```go
// BEFORE
log.Printf("[job_listing_relevance_filter] publish failed for %s: %v", job.ID, err)

// AFTER
w.log.Error("publish failed", err, logger.Str(logger.FieldJobID, job.ID))
```

- [ ] **Step 3: Update imports**

Replace `"log"` with `"github.com/trushilpatel/jobradar/internal/logger"`.

- [ ] **Step 4: Verify**

```bash
cd backend && go build ./internal/worker/job_listing_relevance_filter/...
```

Expected: PASS

---

## Task 11: Update `internal/worker/job_post_analyzer/worker.go`

**Files:**
- Modify: `backend/internal/worker/job_post_analyzer/worker.go`

- [ ] **Step 1: Add logger to Worker struct and constructor**

```go
type Worker struct {
	consumer *kafka.Consumer
	scorer   llm.Scorer
	jobs     store.JobWriter
	ledger   store.SeenLedgerWriter
	snapshot store.SnapshotWriter
	client   *http.Client
	log      logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	scorer llm.Scorer,
	jobs store.JobWriter,
	ledger store.SeenLedgerWriter,
	snapshot store.SnapshotWriter,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer: consumer,
		scorer:   scorer,
		jobs:     jobs,
		ledger:   ledger,
		snapshot: snapshot,
		client:   &http.Client{Timeout: 15 * time.Second},
		log:      log,
	}
}
```

- [ ] **Step 2: Replace log calls in Run**

```go
// BEFORE
log.Printf("[job_post_analyzer] worker started (concurrency=%d)", enrichConcurrency)
// AFTER
w.log.Info("worker started", logger.Int("concurrency", enrichConcurrency))
```

```go
// BEFORE
log.Printf("[job_post_analyzer] malformed message: %v — skipping", err)
// AFTER
w.log.Warn("malformed message, skipping", logger.Err(err))
```

```go
// BEFORE
log.Printf("[job_post_analyzer] received %s — %q", job.ID, job.Title)
// AFTER
w.log.Debug("job received",
	logger.Str(logger.FieldJobID, job.ID),
	logger.Str("title", job.Title),
)
```

- [ ] **Step 3: Replace log calls in enrich — create job-scoped child logger**

At the start of the `enrich` method (or whichever method processes a single job), create a child logger:

```go
jobLog := w.log.With(
	logger.Str(logger.FieldJobID, job.ID),
	logger.Str("title", job.Title),
)
```

Then replace each log call using `jobLog`:

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: fetch content failed: %v", job.ID, err)
// AFTER
jobLog.Error("fetch content failed", err)
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: marshal content failed: %v", job.ID, err)
// AFTER
jobLog.Error("marshal content failed", err)
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: LLM enrich failed: %v", job.ID, err)
// AFTER
jobLog.Error("LLM enrich failed", err)
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: build job failed: %v", job.ID, err)
// AFTER
jobLog.Error("build job failed", err)
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: insert job failed: %v", job.ID, err)
// AFTER
jobLog.Error("insert job failed", err)
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: write snapshot failed: %v", job.ID, err)
// AFTER
jobLog.Warn("write snapshot failed (non-fatal)", logger.Err(err))
```

```go
// BEFORE
log.Printf("[job_post_analyzer] %s: enriched and saved — %q", job.ID, job.Title)
// AFTER
jobLog.Info("enriched and saved")
```

```go
// BEFORE (in markError helper)
log.Printf("[job_post_analyzer] %s: failed to mark error in seen_ledger: %v", jobID, err)
// AFTER
w.log.Error("failed to mark error in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
```

- [ ] **Step 4: Update imports**

Replace `"log"` with `"github.com/trushilpatel/jobradar/internal/logger"`.

- [ ] **Step 5: Verify**

```bash
cd backend && go build ./internal/worker/job_post_analyzer/...
```

Expected: PASS

---

## Task 12: Update API middleware and router

**Files:**
- Modify: `backend/internal/api/middleware/logger.go`
- Modify: `backend/internal/api/router.go`

- [ ] **Step 1: Rewrite middleware/logger.go as a factory**

```go
package middleware

import (
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/internal/logger"
)

// Logger returns an HTTP middleware that logs each request's method, path,
// status code, and duration. Accepts a Logger so the service name and format
// are inherited from the process-wide logger.
func Logger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			log.Info("request",
				logger.Str(logger.FieldMethod, r.Method),
				logger.Str(logger.FieldPath, r.URL.Path),
				logger.Int(logger.FieldStatus, ww.status),
				logger.Dur(logger.FieldDuration, time.Since(start).Round(time.Millisecond)),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the written status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
```

- [ ] **Step 2: Update router.go to accept logger and pass to middleware**

```go
func NewRouter(
	log logger.Logger,
	jobs *handler.JobHandler,
	apps *handler.ApplicationHandler,
	companies *handler.CompanyHandler,
	dashboard *handler.DashboardHandler,
	insights *handler.InsightsHandler,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger(log))
	r.Use(chimiddleware.Recoverer)

	// ... all routes unchanged ...
}
```

Add import: `"github.com/trushilpatel/jobradar/internal/logger"`

- [ ] **Step 3: Verify api package compiles**

```bash
cd backend && go build ./internal/api/...
```

Expected: PASS (cmd/api will fail — fixed in Task 13)

---

## Task 13: Update all `cmd/*/main.go`

**Files:**
- Modify: `backend/cmd/company_scan_scheduler/main.go`
- Modify: `backend/cmd/job_listing_crawler/main.go`
- Modify: `backend/cmd/job_listing_relevance_filter/main.go`
- Modify: `backend/cmd/job_post_analyzer/main.go`
- Modify: `backend/cmd/api/main.go`

**Pattern for all mains:** Call `logger.Init("service-name")` as the first statement after `config.Load()`. Replace `log.Fatalf(...)` with `log.Error(...); os.Exit(1)`. Pass log to all constructors that now require it.

- [ ] **Step 1: Update cmd/company_scan_scheduler/main.go**

```go
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
)

const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicCompanyScanRequest); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	companies := store.NewCompanyStore(database)

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
	)

	enqueuedue(ctx, log, companies, producer, cfg.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, log, companies, producer, cfg.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func enqueuedue(ctx context.Context, log logger.Logger, companies store.CompanyReader, pub kafka.Publisher, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	for _, c := range due {
		payload, err := json.Marshal(scanRequest{CompanyID: c.ID})
		if err != nil {
			log.Error("marshal failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		if err := pub.Publish(ctx, kafka.TopicCompanyScanRequest, payload); err != nil {
			log.Error("publish failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
```

- [ ] **Step 2: Update cmd/job_listing_crawler/main.go**

```go
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_crawler"
)

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	consumer, err := kafka.NewConsumer(cfg, log, "job-listing-crawler", kafka.TopicCompanyScanRequest)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewCompanyScanLogStore(database)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	engine := job_listing_crawler.NewEngine(companies, ledger, scanLog, producer, companyDelay, log)

	log.Info("worker started", logger.Str("topic", kafka.TopicCompanyScanRequest))

	type scanRequest struct {
		CompanyID string `json:"company_id"`
	}

	err = consumer.Poll(ctx, func(_ string, value []byte) error {
		var req scanRequest
		if err := json.Unmarshal(value, &req); err != nil {
			log.Warn("malformed message, skipping", logger.Err(err))
			return nil
		}
		if req.CompanyID == "" {
			log.Warn("message missing company_id, skipping")
			return nil
		}
		engine.RunForCompany(ctx, req.CompanyID)
		return nil
	})

	if err != nil && ctx.Err() == nil {
		log.Error("consumer error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

- [ ] **Step 3: Update cmd/job_listing_relevance_filter/main.go**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_relevance_filter"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	if err := kafka.EnsureTopics(context.Background(), cfg, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	ledger := store.NewSeenLedgerStore(database)
	worker := job_listing_relevance_filter.NewWorker(consumer, producer, scorer, ledger,
		cfg.ScorerBatchSize, cfg.ScorerFlushTimeoutSeconds, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

- [ ] **Step 4: Update cmd/job_post_analyzer/main.go**

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_post_analyzer"
)

func main() {
	cfg := config.Load()
	log := logger.Init("enricher")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg, log, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	jobs := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	snapshot := store.NewSnapshotStore(database)
	worker := job_post_analyzer.NewWorker(consumer, scorer, jobs, ledger, snapshot, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

- [ ] **Step 5: Update cmd/api/main.go**

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/api"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	kafkaCtx := context.Background()
	if err := kafka.EnsureTopics(kafkaCtx, cfg, kafka.TopicCompanyScanRequest); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	jobStore := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	discardStore := store.NewDiscardStore(database)
	snapshotStore := store.NewSnapshotStore(database)
	appStore := store.NewApplicationStore(database)
	companyStore := store.NewCompanyStore(database)
	scanLogStore := store.NewCompanyScanLogStore(database)
	insightsStore := store.NewInsightsStore(database)

	jobHandler := handler.NewJobHandler(jobStore, ledger, discardStore, snapshotStore)
	appHandler := handler.NewApplicationHandler(appStore, jobStore)
	companyHandler := handler.NewCompanyHandler(companyStore, scanLogStore, producer, cfg.ScanIntervalMinutes)
	dashboardHandler := handler.NewDashboardHandler(jobStore, scanLogStore, insightsStore)
	insightsHandler := handler.NewInsightsHandler(insightsStore)

	router := api.NewRouter(log, jobHandler, appHandler, companyHandler, dashboardHandler, insightsHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", logger.Str("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 6: Verify everything compiles**

```bash
cd backend && go build ./...
```

Expected: PASS — zero compilation errors across all packages.

- [ ] **Step 7: Run all tests**

```bash
cd backend && go test ./...
```

Expected: PASS

---

## Task 14: Update `docker-compose.yml`

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add LOG_LEVEL and LOG_FORMAT to each service**

For `scheduler`, `crawler`, `filter`, `enricher`, and `api` — add to each service's `environment` block:

```yaml
LOG_LEVEL: info
LOG_FORMAT: json
```

Example for the `api` service (same pattern for all 5):

```yaml
api:
  environment:
    DB_HOST: postgres
    DB_PORT: "5432"
    KAFKA_BROKERS: kafka:29092
    KAFKA_USER: ""
    KAFKA_PASSWORD: ""
    LOG_LEVEL: info
    LOG_FORMAT: json
```

- [ ] **Step 2: Verify docker-compose syntax**

```bash
docker compose config --quiet
```

Expected: no errors

- [ ] **Step 3: Add LOG_LEVEL and LOG_FORMAT to local .env for pretty dev output**

Add to `backend/.env`:

```
LOG_LEVEL=debug
LOG_FORMAT=pretty
```

---

## Final Verification

- [ ] **Build all binaries**

```bash
cd backend && go build ./...
```

Expected: PASS

- [ ] **Run all tests**

```bash
cd backend && go test ./...
```

Expected: PASS

- [ ] **Smoke test locally with pretty output**

```bash
cd backend && LOG_LEVEL=debug LOG_FORMAT=pretty go run ./cmd/api/main.go
```

Expected: colored, leveled log output like:
```
10:00:00 INF listening addr=:8080 service=api caller=main.go:72
```
