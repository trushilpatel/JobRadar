# Distributed Tracing: OpenTelemetry + Jaeger

**Date:** 2026-04-17  
**Status:** Approved

## Goal

Add end-to-end distributed tracing across all 5 JobRadar services (api, scheduler, crawler, filter, enricher) using the OpenTelemetry Go SDK with Jaeger as the trace backend. A single job processed through the pipeline (Crawler → Filter → Enricher) must produce one unified trace even though it crosses three separate processes via Kafka.

## Approach

Manual `ctx context.Context` threading + `tracing.Start`/`End` at every service and store boundary. This is the same pattern used by Uber (who created Jaeger) and Google internally. No code generation tools, no eBPF — explicit and readable.

## Architecture

```
HTTP Request
  → OTel HTTP middleware (root span auto-created)
    → handler(ctx) → service(ctx) → store(ctx)
      → GORM plugin (child DB spans auto-created)

Kafka Publish
  → traceparent injected into Kafka message headers

Kafka Consume
  → traceparent extracted from headers → ctx restored
    → worker(ctx) → service(ctx) → store(ctx)
      → GORM plugin (child DB spans)
```

Trace context flows through `context.Context` within a process. It crosses process boundaries via W3C `traceparent` headers embedded in Kafka messages.

## New Package: `infra/tracing/`

Single package owned by all workers and services. Three files:

### `tracer.go`

Bootstraps OTel at startup. Sets the global tracer provider so every package can call `otel.Tracer("name")` without dependency injection.

```go
func Init(serviceName, endpoint string) (shutdown func())
```

Called once in each `cmd/*/main.go`. Returns a shutdown function to flush pending spans on graceful exit.

### `span.go`

Two helpers used at every instrumented call site:

```go
func Start(ctx context.Context, name string) (context.Context, trace.Span)
func End(span trace.Span, err error)
```

`End` records the error on the span if non-nil, then closes it. Usage pattern:

```go
func (s *service) List(ctx context.Context, ...) {
    ctx, span := tracing.Start(ctx, "job.service.List")
    defer tracing.End(span, nil)
    ...
}
```

### `kafka.go`

W3C `traceparent` propagation for franz-go (`twmb/franz-go`). Implements `propagation.TextMapCarrier` over `[]kgo.RecordHeader`.

```go
func InjectKafka(ctx context.Context, headers []kgo.RecordHeader) []kgo.RecordHeader
func ExtractKafka(ctx context.Context, headers []kgo.RecordHeader) context.Context
```

## Interface Changes

Every service and store interface adds `ctx context.Context` as the first parameter. This is correct Go practice for any I/O-bound operation regardless of tracing (enables cancellation and deadlines).

### Services (add ctx to all methods)

- `service/job.Service`
- `service/company.Service`
- `service/application.Service`
- `service/insights.Service`
- `service/dashboard.Service`
- `service/scan.Service`

### Store interfaces (add ctx to all methods)

- `service/job`: JobStore, SeenLedgerReader, SeenLedgerWriter, SeenLedgerCompanyReader, DiscardStore, SnapshotWriter
- `service/company`: CompanyStore
- `service/application`: ApplicationStore
- `service/insights`: InsightsStore
- `service/dashboard`: DashboardStore
- `service/scan`: ScanStore, ScanLogStore, ScanQueueStore

### Pipeline interfaces (add ctx to all methods)

- `pipeline/crawler`: CompanyReader, PipelineLedger, ScanRecorder
- `pipeline/filter`: FilterLedger

### Already have ctx (no change needed)

- `pipeline/llm.Scorer` — already correct
- `infra/kafka.Producer.Publish` — already correct
- `infra/kafka.Consumer.Poll` — already correct

## Implementation Pattern Per Method

Every instrumented method follows the same 2-line pattern:

```go
func (s *service) MethodName(ctx context.Context, args...) (returns...) {
    ctx, span := tracing.Start(ctx, "package.type.MethodName")
    defer tracing.End(span, nil)

    result, err := s.dependency.Call(ctx, args...)
    if err != nil {
        tracing.End(span, err)  // record error before returning
        return nil, err
    }
    return result, nil
}
```

Span naming convention: `{package}.{type}.{Method}` — e.g. `job.service.List`, `job.store.GetJobs`.

## Infrastructure Changes

### `infra/db/db.go`

Register GORM OTel plugin after opening DB. This automatically creates a child span for every SQL query with the query text and table name attached.

```go
db.Use(otelgorm.NewPlugin())
```

### `infra/kafka/producer.go`

Inject trace context into Kafka headers before every publish:

```go
record.Headers = tracing.InjectKafka(ctx, record.Headers)
```

### `infra/kafka/consumer.go`

Extract trace context from Kafka headers before processing each record:

```go
ctx = tracing.ExtractKafka(ctx, record.Headers)
```

### `api/router.go`

Mount OTel HTTP middleware as the outermost middleware. Creates a root span for every incoming request using the chi route pattern as the span name.

```go
r.Use(otelhttp.NewMiddleware("jobradar-api"))
```

### `cmd/*/main.go` (all 5 binaries)

Add tracing init after logger init, before any other setup:

```go
shutdown := tracing.Init(cfg.ServiceName, cfg.JaegerEndpoint)
defer shutdown()
```

## Config

Add to `config/config.go`:

- `JAEGER_ENDPOINT` — OTLP HTTP endpoint, default `http://localhost:4318`
- `SERVICE_NAME` — already derivable from the logger service name, reuse it

## Docker Compose Changes

### Add Jaeger all-in-one

Single container. Stores traces in memory (sufficient for development). Exposes:

- Port `16686` — Jaeger UI
- Port `4318` — OTLP HTTP receiver (apps send traces here)

### Uncomment Prometheus + Grafana

Both are already configured but commented out. Uncomment them. Prometheus already scrapes all 5 service `/metrics` endpoints. Grafana already has a Prometheus datasource provisioned.

### Add env vars to all services

`JAEGER_ENDPOINT: http://jaeger:4318` added to api, scheduler, crawler, filter, enricher.

## Go Dependencies to Add

```
go.opentelemetry.io/otel
go.opentelemetry.io/otel/trace
go.opentelemetry.io/otel/sdk/trace
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go.opentelemetry.io/otel/propagation
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go.opentelemetry.io/contrib/instrumentation/gorm.io/gorm/otelgorm
```

## What You See in Jaeger After Implementation

A single job through the full pipeline:

```
[Crawler: crawler.engine.RunForCompany]          ~2s
  ├── [crawler.store.GetActiveCompanies]          ~5ms
  │     └── SELECT companies WHERE ...           ~4ms  ← GORM auto
  └── [kafka publish: job_listings.raw]           ~2ms

[Filter: filter.worker.processBatch]             ~800ms
  ├── [llm.scorer.ScoreBatch]                    ~700ms
  └── [filter.store.UpdateSeenReasonBatch]        ~5ms

[Enricher: analyzer.worker.processJob]           ~1.2s
  ├── [llm.scorer.Enrich]                        ~900ms
  ├── [job.store.InsertJob]                       ~20ms
  │     └── INSERT INTO job_feed ...             ~18ms  ← GORM auto
  └── [snapshot.InsertSnapshot]                  ~8ms

[API: GET /jobs]                                 ~45ms
  └── [job.service.List]                         ~43ms
        └── [job.store.GetJobs]                  ~40ms
              └── SELECT job_feed WHERE ...      ~38ms  ← GORM auto
```

## Files Touched

| Category                             | Count |
| ------------------------------------ | ----- |
| New `infra/tracing/` files           | 3     |
| Service interfaces + implementations | ~30   |
| Store interfaces + implementations   | ~15   |
| Pipeline workers                     | 3     |
| Infra (db, kafka, router)            | 4     |
| `cmd/*/main.go`                      | 5     |
| `docker-compose.yml`                 | 1     |
| `config/config.go`                   | 1     |
| `go.mod` / `go.sum`                  | 1     |

**~63 files total.** All changes follow the same mechanical pattern.
