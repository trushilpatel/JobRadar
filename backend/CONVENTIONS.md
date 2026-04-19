# Core conventions

## Store pattern (the most important rule)

Workers never hold `*gorm.DB`. They hold store interfaces.
Store constructors always return the interface, not the concrete type.

```go
// correct
func NewJobStore(db *gorm.DB) JobStore { return &jobStore{db: db} }

// wrong
func NewJobStore(db *gorm.DB) *jobStore { return &jobStore{db: db} }
```

## Interface-first everywhere

Same rule applies to Kafka, LLM, and Logger:
- Workers depend on `platform/kafka.Publisher`, not `*kafka.Producer`
- Filter worker depends on `service/llm/services.JobsRelevanceFilter`, not `*geminiScorer`
- Analyzer worker depends on `service/llm/services.JobPostEnricher`, not `*geminiScorer`
- Workers depend on `platform/logger.Logger`, not `*logger.zerologLogger`

This means any worker can be tested by passing a fake implementation. Use `logger.NewNoop()` in tests to suppress output without any setup.

## Logging

All packages use `platform/logger.Logger` — never stdlib `log`. Workers receive the logger via constructor injection.

```go
// correct — inject at construction, use structured fields
type Engine struct {
    log logger.Logger
}

func NewEngine(..., log logger.Logger) *Engine { ... }

// in methods — use leveled calls with field constants
e.log.Info("scan complete",
    logger.Int(logger.FieldFetched, fetched),
    logger.Dur(logger.FieldDuration, elapsed),
)

// for per-job tracing — create child logger with baked-in IDs
jobLog := w.log.With(
    logger.Str(logger.FieldJobID, job.ID),
    logger.Str(logger.FieldCompany, job.CompanyID),
)
```

**Log levels:**
- `Debug` — per-item events, sleep intervals, routine steps (suppressed at `LOG_LEVEL=info`)
- `Info` — meaningful milestones: worker started, cycle complete, job saved
- `Warn` — recoverable issues: malformed messages, flush timeouts, rate limit waits
- `Error` — actual failures: DB write failed, LLM failed, publish failed

**Field keys** must use constants from `platform/logger/fields.go` — never raw strings:

```go
logger.FieldJobID    // "job_id"
logger.FieldCompany  // "company_id"
logger.FieldFetched  // "fetched"
```

## Model naming

All models have an explicit `TableName()` method. Never rely on GORM auto-naming.

| Struct | Table |
|---|---|
| `Company` | `companies` |
| `JobSeenLedger` | `job_pipeline_ledger` |
| `CompanyScanLog` | `company_scan_log` |
| `Job` | `job_feed` |
| `JobSnapshot` | `job_analytics` |
| `JobApplication` | `job_application_tracker` |
| `Discarded` | `job_dismissal_log` |

## Constants

All constrained values live in `model/constants.go`. Use them — never hardcode strings.

```go
model.JobStatusNew        // "new"
model.SeenReasonFetch     // "fetch"
model.StagePhoneScreen    // "phone_screen"
```
