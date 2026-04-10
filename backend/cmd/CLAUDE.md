# cmd/ — Binary entry points

Each binary wires dependencies together and starts the process. No business logic lives here.

## Wiring pattern

Every `main.go` follows this order:
1. Load config
2. **Init logger** — `log := logger.Init("service-name")` — must be first so startup errors are structured
3. Open DB
4. Call `kafka.EnsureTopics` (if producing) — **before** creating the producer
5. Create Kafka producer/consumer (pass `log` to `kafka.NewConsumer`)
6. Create LLM scorer (if needed, pass `Log: log` in `ScorerOptions`)
7. Create stores
8. Create worker/handler (pass `log` to constructor)
9. Set up signal context
10. Start and block

Startup failures use `log.Error(...)` + `os.Exit(1)` — never `log.Fatalf`.

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
| `company_scan_scheduler` | inserts into `company_scan_queue` (DB) | checks DB every 1 min, enqueues companies due per `SCAN_INTERVAL_MINUTES` config |
| `job_listing_crawler` | polls `company_scan_queue` (DB), publishes to `job_listings.raw` | polls every 10s; `FOR UPDATE SKIP LOCKED` claim is safe for concurrent instances |
| `job_listing_relevance_filter` | consumes `job_listings.raw`, publishes to `job_listings.filtered` | event-driven |
| `job_post_analyzer` | consumes `job_listings.filtered` | event-driven |
| `api` | `:8080` | always on |
| `migrate` | — | run once manually |
