# platform/tracing/ — OpenTelemetry tracing

## Init

Call `tracing.Init(serviceName, endpoint)` once in `main()` after loading config. Returns a shutdown function — always defer it:

```go
shutdown := tracing.Init("crawler", cfg.Observability.JaegerEndpoint)
defer shutdown()
```

`endpoint` is the OTLP HTTP endpoint (e.g. `"http://jaeger:4318"`). Sets the global OTel tracer provider and W3C propagator.

## Spans

Use `tracing.Start` / `tracing.End` for all service and store methods. The pattern requires a named return error:

```go
func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
    ctx, span := tracing.Start(ctx, "job.service.Get")
    defer tracing.End(span, &err)
    // ...
}
```

- `Start` creates a child span of whatever span is in `ctx`; returns the new `ctx` — pass it to all downstream calls
- `End` closes the span and records the error if non-nil; the pointer captures the final named return value

**Naming convention:** `{package}.{type}.{Method}` — e.g. `job.store.GetJobs`, `scan.crawlerservice.RecordScanResult`

## Kafka propagation

Inject the W3C traceparent into Kafka message headers before publishing:

```go
headers = tracing.InjectKafka(ctx, headers)
producer.Publish(ctx, topic, key, value, headers)
```

Extract the traceparent from headers after consuming:

```go
ctx = tracing.ExtractKafka(ctx, record.Headers)
// now ctx carries the upstream span as parent
```

This links spans across service boundaries in Jaeger.

## Configuration

| Env var           | Default                 |
| ----------------- | ----------------------- |
| `JAEGER_ENDPOINT` | `http://localhost:4318` |

The tracing package is a no-op if `Init` is not called — the global OTel provider stays as the default no-op provider.
