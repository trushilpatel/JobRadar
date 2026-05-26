# platform/tracing/ — OpenTelemetry tracing

## Init

Call `tracing.Init(serviceName, endpoint)` once in `main()` after loading config. Returns a shutdown function — always defer it.

```go
shutdown := tracing.Init("crawler", cfg.Observability.OTelEndpoint)
defer shutdown()
```

`endpoint` is the OTLP HTTP endpoint (e.g. `http://obs-alloy.jobradar-local.svc.cluster.local:4318` — Grafana Alloy in-cluster, which forwards to Grafana Cloud Tempo). Sets the global OTel tracer provider and W3C propagator.

## Spans

Use `tracing.Start` / `tracing.End` for all service and store methods. The pattern requires a **named return error**:

```go
func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
    ctx, span := tracing.Start(ctx, "job.service.Get")
    defer tracing.End(span, &err)
    // …
}
```

- `Start` creates a child span of whatever span is in `ctx`; returns the new `ctx` — pass it to all downstream calls
- `End` closes the span and records the error if non-nil; the pointer captures the final named return value

**Naming convention:** `{package}.{type}.{Method}` — e.g. `job.store.GetJobs`, `scan.crawlerservice.RecordScanResult`.

## NATS propagation

Inject the W3C traceparent into NATS headers before publishing, extract after consuming. This links spans across service boundaries in Grafana Cloud Tempo.

```go
// publish
headers = tracing.InjectNATS(ctx, headers)
producer.Publish(ctx, subject, payload, headers)

// consume
ctx = tracing.ExtractNATS(ctx, msg.Header)
```

## Configuration

| Env var                       | Default                 |
| ----------------------------- | ----------------------- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` |

The package is a no-op if `Init` is not called — the global OTel provider stays as the default no-op provider.
