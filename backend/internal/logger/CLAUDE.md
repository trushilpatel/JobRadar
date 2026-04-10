# internal/logger/ — Structured logging

## Package layout

```
logger.go      ← Logger interface (the only type workers depend on)
fields.go      ← Field type, helper constructors, field key constants
zerolog.go     ← zerolog implementation + Init() + NewWithWriter() for tests
noop.go        ← NoopLogger — use in unit tests via logger.NewNoop()
```

## Usage

Call `logger.Init("service-name")` once at the top of `main()`. Pass the returned `Logger` to every constructor that needs it.

```go
log := logger.Init("crawler")
engine := job_listing_crawler.NewEngine(..., log)
```

## Log levels

| Level | When |
|---|---|
| `Debug` | Per-item events, sleep intervals — suppressed at `LOG_LEVEL=info` |
| `Info` | Meaningful milestones: worker started, cycle complete, job saved |
| `Warn` | Recoverable issues: malformed messages, rate limit waits, flush timeouts |
| `Error` | Actual failures: DB write, LLM call, Kafka publish |

## Field helpers

Always use the constructor functions — never `Field{Key: "...", Value: ...}` directly:

```go
logger.Str(key, val string) Field
logger.Int(key string, val int) Field
logger.Dur(key string, d time.Duration) Field
logger.Bool(key string, val bool) Field
logger.Err(err error) Field   // adds error under FieldError key
```

## Field key constants

Always use constants — never raw strings. Consistent keys are required for future Loki queries:

```go
logger.FieldService   // "service"
logger.FieldCompany   // "company_id"
logger.FieldJobID     // "job_id"
logger.FieldFetched   // "fetched"
logger.FieldNew       // "new"
logger.FieldDuration  // "duration"
logger.FieldBatch     // "batch"
logger.FieldRelevant  // "relevant"
logger.FieldError     // "error"
```

## Child loggers (pipeline correlation)

Use `With()` at the start of a job-processing loop to bake `job_id` and `company_id` into every subsequent line. This makes it possible to trace one job through all services in future Loki queries.

```go
jobLog := w.log.With(
    logger.Str(logger.FieldJobID, job.ID),
    logger.Str(logger.FieldCompany, job.CompanyID),
)
jobLog.Info("enrichment started")
jobLog.Error("LLM failed", err)  // job_id + company_id auto-included
```

## Configuration

| Env var | Values | Default | Notes |
|---|---|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | Set `debug` locally |
| `LOG_FORMAT` | `json`, `pretty` | `json` | Set `pretty` locally for colored output |

Local `.env` should have `LOG_LEVEL=debug` and `LOG_FORMAT=pretty`.
Docker Compose services use `LOG_LEVEL=info` and `LOG_FORMAT=json` (Loki-ready).

## Switching backends

The `Logger` interface is the only type call sites depend on. To swap zerolog for another backend:
1. Write a new implementation file (e.g. `slog.go`) implementing `Logger`
2. Update `Init()` in `zerolog.go` to return the new impl
3. No call sites change
