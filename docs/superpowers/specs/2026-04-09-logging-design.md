# Logging System Design

**Date:** 2026-04-09  
**Project:** JobRadar  
**Scope:** Phase A — Structured logging with log levels across all backend services (foundation for future Grafana Loki integration)

---

## Problem

All 5 workers and the API use Go's stdlib `log.Printf` / `log.Fatalf` — completely flat, no log levels, no structured fields. Every message is the same priority. Manual prefixes like `[job_listing_crawler]` are the only "structure". When viewed via Docker Desktop UI, all services produce a wall of undifferentiated noise with no searchability or cross-service traceability.

**Specific pain points:**
- No log levels — routine sleeps/per-item events drown out errors
- No structured fields — can't filter by `company_id` or `job_id`
- No cross-pipeline correlation — can't trace one job from crawler → filter → enricher
- Manual `[service_name]` prefixes are fragile and inconsistent

---

## Goals

- Reduce noise: set `LOG_LEVEL=info` in prod and debug lines disappear
- Searchability: structured key-value fields on every log line
- Auto caller info: replaces manual `[service_name]` prefixes
- Pipeline correlation: `job_id` + `company_id` on every relevant line
- Loki-ready: JSON output format so future Loki integration is plug-and-play
- Swappable: thin abstraction layer so the backend (zerolog) can be replaced without touching call sites

---

## Non-Goals (Phase A)

- Grafana / Loki / Prometheus setup (Phase B)
- Log shipping or aggregation
- Metrics or tracing

---

## Library Choice: `rs/zerolog`

Selected over stdlib `slog` and `uber-go/zap` for its ergonomic chained API and clean output. Single external dependency.

```go
log.Info().Str("company", name).Int("fetched", 42).Dur("duration", d).Msg("scan complete")
```

---

## Architecture

### Package: `internal/logger/`

```
internal/logger/
├── logger.go      ← Logger interface, Field type, helper constructors
├── fields.go      ← Standardized field name constants
├── zerolog.go     ← zerolog implementation, Init(), env config
└── noop.go        ← no-op implementation for tests
```

### Logger Interface (`logger.go`)

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, err error, fields ...Field)
    With(fields ...Field) Logger  // returns child logger with baked-in fields
}

type Field struct {
    Key   string
    Value any
}

// Field constructors
func Str(key, val string) Field
func Int(key string, val int) Field
func Err(err error) Field
func Dur(key string, d time.Duration) Field
func Bool(key string, val bool) Field
```

### Standardized Field Names (`fields.go`)

```go
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
)
```

All services must use these constants as field keys — never raw strings — to ensure consistent naming for future Loki queries.

### Initialization (`zerolog.go`)

```go
func Init(service string) Logger
```

Called once in each binary's `main.go`. Reads two env vars:

| Env var | Values | Default |
|---|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `LOG_FORMAT` | `json`, `pretty` | `json` |

- `json`: Raw JSON to stdout — machine-readable, Loki-ready
- `pretty`: Colored human-readable output via `zerolog.ConsoleWriter` — for local dev

Automatically bakes in:
- `service` field (from argument)
- `caller` field (file:line via `zerolog.CallerFieldName`) — replaces all manual `[service]` prefixes
- `time` field (RFC3339)

### No-op Implementation (`noop.go`)

Silent logger that satisfies the `Logger` interface. Used in unit tests to suppress log output without any setup.

---

## Log Level Assignment

### Level Semantics

| Level | When to use |
|---|---|
| `DEBUG` | Per-item events, routine internal steps, sleep intervals |
| `INFO` | Meaningful milestones and summaries (worker start, cycle complete, job saved) |
| `WARN` | Recoverable issues — skipped items, timeouts, rate limit waits |
| `ERROR` | Actual failures requiring attention — DB write failed, LLM failed, publish failed |

### Per-Service Mapping

**company_scan_scheduler:**
- `INFO`: worker started, N companies due for scan, company queued, shutting down
- `DEBUG`: individual publish confirmations
- `ERROR`: marshal failed, publish failed, query failed

**job_listing_crawler:**
- `INFO`: scan cycle started, scan complete (fetched/new/duration summary per company)
- `DEBUG`: sleeping between companies, triggered scan for company
- `WARN`: company inactive (skipped)
- `ERROR`: load companies failed, seen_ledger check failed, publish failed, scan log write failed

**job_listing_relevance_filter:**
- `INFO`: worker started, batch result (batch/relevant/irrelevant counts)
- `DEBUG`: flush timeout triggered, shutdown flush
- `WARN`: malformed message skipped
- `ERROR`: ScoreBatch failed, mark error batch failed, mark irrelevant failed, publish failed

**job_post_analyzer:**
- `INFO`: worker started, job enriched and saved
- `DEBUG`: job received (per message)
- `WARN`: malformed message skipped
- `ERROR`: fetch content failed, LLM enrich failed, build job failed, insert job failed, write snapshot failed, mark error failed

**api:**
- `INFO`: server listening, shutting down
- `ERROR`: server error, startup failures

**api middleware (HTTP logger):**
- `INFO`: request log (method, path, status, duration) — already one line per request, keep as INFO

---

## Pipeline Correlation

For workers that process individual jobs, create a child logger early in the processing function with `job_id` and `company_id` baked in:

```go
jobLog := log.With(
    logger.Str(logger.FieldJobID, job.ID),
    logger.Str(logger.FieldCompany, job.CompanyID),
)
jobLog.Info("enrichment started")
jobLog.Error("LLM failed", err)
// Every line automatically carries job_id and company_id
```

This enables future Loki query: `{service="enricher"} | json | job_id = "abc123"` to see the full journey.

---

## Configuration

### Local dev (`.env`):
```
LOG_LEVEL=debug
LOG_FORMAT=pretty
```

### Docker Compose services (prod-like):
```yaml
environment:
  LOG_LEVEL: info
  LOG_FORMAT: json
```

Add these env vars to all 5 service definitions in `docker-compose.yml`.

---

## Migration Plan

1. Add `rs/zerolog` to `go.mod`
2. Create `internal/logger/` package (interface, fields, zerolog impl, noop)
3. Update each `cmd/*/main.go` — replace `log.Fatalf` startup errors, call `logger.Init`
4. Update each worker — replace all `log.Printf` calls with leveled structured calls
5. Update API middleware HTTP logger
6. Add `LOG_LEVEL` and `LOG_FORMAT` env vars to `docker-compose.yml`
7. Remove all manual `[service_name]` string prefixes (now automatic via caller)

---

## Phase B Preview (Future)

When ready to add Grafana Loki:
- Add `grafana`, `loki`, and `promtail` (or Grafana Alloy) services to `docker-compose.yml`
- Promtail tails Docker container stdout and ships to Loki
- Loki ingests the JSON lines natively — no parsing rules needed (fields already structured)
- Grafana connects to Loki as datasource
- Query example: `{service="crawler"} | json | level = "error"` shows all crawler errors

No code changes required in Phase B — the logging foundation is already Loki-ready.
