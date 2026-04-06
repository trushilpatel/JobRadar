# cmd/ — Binary entry points

Each binary wires dependencies together and starts the process. No business logic lives here.

## Wiring pattern

Every `main.go` follows this order:
1. Load config
2. Open DB
3. Call `kafka.EnsureTopics` (if producing) — **before** creating the producer
4. Create Kafka producer/consumer
5. Create LLM scorer (if needed)
6. Create stores
7. Create worker/handler
8. Set up signal context
9. Start and block

## EnsureTopics rule

`kafka.EnsureTopics` must be called before `kafka.NewProducer` in any binary that publishes to Kafka.
Kafka does not auto-create topics with SASL enabled. Skipping this causes `UNKNOWN_TOPIC_OR_PARTITION` errors.

## Signal handling

All binaries use `signal.NotifyContext` for graceful shutdown on SIGINT/SIGTERM:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

The context is passed into workers so in-flight jobs drain before exit.

## Binaries

| Binary | Port/topic | Interval |
|---|---|---|
| `scheduler` | publishes to `company.scan_job` | checks DB every 1 min, enqueues companies due per `SCAN_INTERVAL_MINUTES` config |
| `scan` | consumes `company.scan_job`, publishes to `jobs.raw` | event-driven (triggered by scheduler) |
| `scorer` | consumes `jobs.raw`, publishes to `jobs.raw_detail` | event-driven |
| `enricher` | consumes `jobs.raw_detail` | event-driven |
| `api` | `:8080` | always on |
| `migrate` | — | run once manually |
