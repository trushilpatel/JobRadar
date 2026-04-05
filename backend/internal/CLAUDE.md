# internal/ — Core conventions

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

Same rule applies to Kafka and LLM:
- Workers depend on `kafka.Publisher`, not `*kafka.Producer`
- Workers depend on `llm.Scorer`, not `*anthropicScorer`

This means any worker can be tested by passing a fake implementation.

## Model naming

All models have an explicit `TableName()` method. Never rely on GORM auto-naming.

| Struct | Table |
|---|---|
| `Company` | `companies` |
| `JobSeenLedger` | `job_seen_ledger` |
| `JobScanLog` | `job_scan_log` |
| `Job` | `job` |
| `JobApplication` | `job_application` |
| `Discarded` | `job_discarded` |

## Constants

All constrained values live in `model/constants.go`. Use them — never hardcode strings.

```go
model.JobStatusNew        // "new"
model.SeenReasonFetch     // "fetch"
model.StagePhoneScreen    // "phone_screen"
```
