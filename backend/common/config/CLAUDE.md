# config/ — Application configuration

`config.Load()` reads a `.env` file (via godotenv) then falls back to environment variables. Called once in each `main.go` before any other setup.

## Environment variables

### Database

| Env var       | Default     | Notes                 |
| ------------- | ----------- | --------------------- |
| `DB_HOST`     | `localhost` |                       |
| `DB_PORT`     | `5432`      |                       |
| `DB_USER`     | `postgres`  |                       |
| `DB_PASSWORD` | `postgres`  |                       |
| `DB_NAME`     | `jobradar`  |                       |
| `DB_SSLMODE`  | `disable`   | Set `require` in prod |

### Kafka

| Env var          | Default          | Notes                |
| ---------------- | ---------------- | -------------------- |
| `KAFKA_BROKERS`  | `localhost:9092` | Comma-separated list |
| `KAFKA_USER`     | `""`             | SASL username        |
| `KAFKA_PASSWORD` | `""`             | SASL password        |

### Redis (LLM rate limiting + cooldown)

| Env var      | Default          | Notes                   |
| ------------ | ---------------- | ----------------------- |
| `REDIS_ADDR` | `localhost:6379` | host:port               |
| `REDIS_DB`   | `0`              | logical database number |

### LLM cooldown

| Env var                     | Default | Notes                         |
| --------------------------- | ------- | ----------------------------- |
| `LLM_COOLDOWN_BASE_SECONDS` | `60`    | Base backoff duration          |
| `LLM_COOLDOWN_MAX_SECONDS`  | `3600`  | Cap on backoff                 |
| `LLM_COOLDOWN_JITTER`       | `0.2`   | ± fraction, must be in [0, 1]  |

### LLM (shared base)

| Env var        | Default                         | Notes                                             |
| -------------- | ------------------------------- | ------------------------------------------------- |
| `LLM_PROVIDER` | `gemini`                        | Only `gemini` supported currently                 |
| `LLM_API_KEY`  | `""`                            | Required                                          |
| `LLM_MODEL`    | `gemini-3.1-flash-lite-preview` | Fallback model if per-worker model unset          |
| `TARGET_ROLES` | `Software Engineer`             | Comma-separated; used in relevance scoring prompt |
| `YEARS_EXP`    | `0`                             | Used in relevance scoring prompt                  |

### Filter worker

| Env var                        | Default                   | Notes                                           |
| ------------------------------ | ------------------------- | ----------------------------------------------- |
| `FILTER_LLM_MODEL`             | falls back to `LLM_MODEL` | Override model for relevance filter             |
| `FILTER_BATCH_SIZE`            | `50`                      | Jobs per LLM batch                              |
| `FILTER_FLUSH_TIMEOUT_SECONDS` | `30`                      | Max wait to fill a batch before flushing        |
| `FILTER_LLM_PARALLELISM`       | `5`                       | Concurrent LLM calls                            |
| `FILTER_RPM`                   | `15`                      | Rate limit: requests per minute (0 = unlimited) |
| `FILTER_RPD`                   | `1500`                    | Rate limit: requests per day                    |
| `FILTER_TPM`                   | `1000000`                 | Rate limit: input tokens per minute             |
| `FILTER_LLM_SLOTS`             | `""`                      | JSON array of `{key_id, api_key, model, rpm, rpd, tpm_in}`. If set, overrides single-slot config. |

### Enricher worker

| Env var                   | Default                                             | Notes                                                   |
| ------------------------- | --------------------------------------------------- | ------------------------------------------------------- |
| `ENRICHER_LLM_MODEL`      | `gemma-4-31b-it` (falls back to `LLM_MODEL` if set) | Override model for job enrichment                       |
| `ENRICHER_CONCURRENCY`    | `5`                                                 | Concurrent enrichment goroutines                        |
| `DISCARD_JOB_WITH_EXP_GT` | `0`                                                 | Drop jobs requiring more than N years exp; 0 = disabled |
| `ENRICHER_RPM`            | `15`                                                | Rate limit: requests per minute                         |
| `ENRICHER_RPD`            | `1500`                                              | Rate limit: requests per day                            |
| `ENRICHER_TPM`            | `1000000`                                           | Rate limit: input tokens per minute                     |
| `ENRICHER_LLM_SLOTS`      | `""`                                                | JSON array of `{key_id, api_key, model, rpm, rpd, tpm_in}`. Same shape as `FILTER_LLM_SLOTS`. |

### Crawler worker

| Env var                      | Default | Notes                                         |
| ---------------------------- | ------- | --------------------------------------------- |
| `SCAN_COMPANY_DELAY_SECONDS` | `15`    | Delay between scanning companies in one cycle |

### Google scraper (within crawler)

| Env var                     | Default    | Notes                                            |
| --------------------------- | ---------- | ------------------------------------------------ |
| `GOOGLE_SCRAPER_MIN_DELAY`  | `120000`   | Min delay between page loads (ms)                |
| `GOOGLE_SCRAPER_MAX_DELAY`  | `240000`   | Max delay between page loads (ms)                |
| `GOOGLE_SCRAPER_TIMEOUT_MS` | `14400000` | Total scrape timeout per company (ms)            |
| `GOOGLE_SCRAPER_MAX_PAGES`  | `0`        | Max pages per company; 0 = unlimited             |
| `GOOGLE_SCRAPER_ENRICH_MIN` | `60000`    | Min delay when loading individual job pages (ms) |
| `GOOGLE_SCRAPER_ENRICH_MAX` | `120000`   | Max delay when loading individual job pages (ms) |

### Scheduler

| Env var                 | Default | Notes                                              |
| ----------------------- | ------- | -------------------------------------------------- |
| `SCAN_INTERVAL_MINUTES` | `1440`  | How often each company is re-scanned (24h default) |

### Observability

| Env var           | Default                 | Notes                          |
| ----------------- | ----------------------- | ------------------------------ |
| `METRICS_ADDR`    | `:9090`                 | Prometheus scrape address      |
| `JAEGER_ENDPOINT` | `http://localhost:4318` | OTLP HTTP endpoint for tracing |

## Usage

```go
cfg := config.Load()
cfg.DB.DSN()               // returns formatted PostgreSQL DSN
cfg.Kafka.Brokers          // string
cfg.Filter.BatchSize       // int
cfg.Enricher.MaxExpYears   // int (DISCARD_JOB_WITH_EXP_GT)
```
