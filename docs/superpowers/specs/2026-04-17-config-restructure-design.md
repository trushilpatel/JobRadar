# Config Restructure Design

**Date:** 2026-04-17  
**Approach:** Nested sub-configs in root `config` package (Option A)

## Problem

The flat `Config` struct mixes DB, Kafka, LLM, per-worker, and observability fields. Infra packages (`db.Open`, `kafka.NewProducer`, etc.) take `*config.Config` but use only 2–3 fields — they're over-coupled.

## Design

### New `Config` shape

```go
type Config struct {
    DB            DatabaseConfig
    Kafka         KafkaConfig
    LLM           LLMConfig
    Filter        FilterConfig
    Enricher      EnricherConfig
    Crawler       CrawlerConfig
    Scheduler     SchedulerConfig
    Observability ObservabilityConfig
}
```

### Sub-structs

```go
type DatabaseConfig struct {
    Host, Port, User, Password, Name, SSLMode string
}
func (c DatabaseConfig) DSN() string

type KafkaConfig struct {
    Brokers, User, Password string
}

type LLMConfig struct {
    Provider    string
    APIKey      string
    BaseModel   string
    TargetRoles []string
    YearsExp    int
}

type RateLimits struct {
    RPM, RPD, TPMIn int
}

type FilterConfig struct {
    Model               string
    BatchSize           int
    FlushTimeoutSeconds int
    LLMParallelism      int
    RateLimits          RateLimits
}

type EnricherConfig struct {
    Model       string
    Concurrency int
    MaxExpYears int
    RateLimits  RateLimits
}

type CrawlerConfig struct {
    CompanyDelaySeconds int
    Google              GoogleScraperConfig
}

type GoogleScraperConfig struct {
    MinDelayMs, MaxDelayMs, TimeoutMs int
    MaxPages                          int
    EnrichMinMs, EnrichMaxMs          int
}

type SchedulerConfig struct {
    ScanIntervalMinutes int
}

type ObservabilityConfig struct {
    MetricsAddr    string
    JaegerEndpoint string
}
```

### Key decisions

- `DSN()` moves from `*Config` to `DatabaseConfig`
- `LLMRateLimits` renamed to `RateLimits` (prefix is redundant inside `FilterConfig`/`EnricherConfig`)
- `MaxExpYears` moves into `EnricherConfig` — only used by the analyzer worker
- `TargetRoles` + `YearsExp` stay in `LLMConfig` — both filter and enricher pass them to `llm.ScorerOptions`
- `GoogleScraperConfig` nested inside `CrawlerConfig`

## Infra changes

Infra functions accept their specific sub-struct instead of `*config.Config`:

| Function             | Old signature    | New signature           |
| -------------------- | ---------------- | ----------------------- |
| `db.Open`            | `*config.Config` | `config.DatabaseConfig` |
| `kafka.NewProducer`  | `*config.Config` | `config.KafkaConfig`    |
| `kafka.NewConsumer`  | `*config.Config` | `config.KafkaConfig`    |
| `kafka.EnsureTopics` | `*config.Config` | `config.KafkaConfig`    |
| `kafka.NewLagReader` | `*config.Config` | `config.KafkaConfig`    |

`tracing.Init` and `metrics.Serve` already take plain strings — no change needed.

## Worker changes

Each worker `main.go` passes the relevant sub-struct(s):

| Worker                         | Sub-configs used                                                      |
| ------------------------------ | --------------------------------------------------------------------- |
| `job_listing_relevance_filter` | `cfg.DB`, `cfg.Kafka`, `cfg.LLM`, `cfg.Filter`, `cfg.Observability`   |
| `job_post_analyzer`            | `cfg.DB`, `cfg.Kafka`, `cfg.LLM`, `cfg.Enricher`, `cfg.Observability` |
| `job_listing_crawler`          | `cfg.DB`, `cfg.Kafka`, `cfg.Crawler`, `cfg.Observability`             |
| `company_scan_scheduler`       | `cfg.DB`, `cfg.Scheduler`, `cfg.Observability`                        |
| `api`                          | `cfg.DB`, `cfg.Kafka`, `cfg.Observability`                            |

## Out of scope

- `llm.ScorerOptions` — unchanged, config feeds values into it at startup
- Any pipeline/store/service logic — pure config + infra wiring change
