# config/ — Application configuration

`config.Load()` reads a `.env` file (via godotenv) then falls back to environment variables. Called once in each `main.go` before any other setup.

## Environment variables

### Database

Database is **Supabase-hosted PostgreSQL** — there is no local postgres container. All services connect to the Supabase pooler via `backend/.env`.

| Env var       | Example value                                    | Notes                        |
| ------------- | ------------------------------------------------ | ---------------------------- |
| `DB_HOST`     | `aws-1-us-east-2.pooler.supabase.com`            | Supabase pooler host         |
| `DB_PORT`     | `5432`                                           |                              |
| `DB_USER`     | `postgres.<project-ref>`                         | Supabase pooler user format  |
| `DB_PASSWORD` | `<supabase-db-password>`                         |                              |
| `DB_NAME`     | `postgres`                                       | Supabase default DB name     |
| `DB_SSLMODE`  | `require`                                        | Always `require` for Supabase|

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

### LLM slots (multi-key / multi-model routing)

Slots are required — the factory errors at startup if none are configured. Precedence (first non-empty wins):

1. **YAML file** — `LLM_SLOTS_CONFIG=/path/to/llm-slots.yaml`. See `config/llm-slots.example.yaml`. `${VAR}` in the file is expanded from the process env at load time so API keys stay in `.env`.
2. **JSON env vars** — `FILTER_LLM_SLOTS` / `ENRICHER_LLM_SLOTS` as a JSON array of `{key_id, api_key, model, rpm, rpd, tpm_in}`.

| Env var             | Default | Notes                                                       |
| ------------------- | ------- | ----------------------------------------------------------- |
| `LLM_SLOTS_CONFIG`  | `""`    | Path to YAML slots file. When set, wins over JSON env vars. |

### LLM (shared)

| Env var        | Default             | Notes                                             |
| -------------- | ------------------- | ------------------------------------------------- |
| `LLM_PROVIDER` | `gemini`            | Only `gemini` supported currently                 |
| `TARGET_ROLES` | `Software Engineer` | Comma-separated; used in relevance scoring prompt |
| `YEARS_EXP`    | `0`                 | Used in relevance scoring prompt                  |

### Filter worker

| Env var                            | Default | Notes                                                            |
| ---------------------------------- | ------- | ---------------------------------------------------------------- |
| `FILTER_BATCH_SIZE`                | `50`    | Jobs per individual LLM ScoreBatch call                          |
| `FILTER_FLUSH_TIMEOUT_SECONDS`     | `30`    | Broker `FetchMaxWait` — accumulate trickle traffic at the broker |
| `FILTER_LLM_CALL_INTERVAL_SECONDS` | `5`     | Sleep between successive LLM calls within one fetch              |
| `FILTER_LLM_SLOTS`                 | `""`    | JSON array of slot configs                                       |

### Enricher worker

Two-stage pipeline: parallel HTTP fetches (Stage 1) → LLM `EnrichBatch` per group (Stage 2).

| Env var                              | Default | Notes                                                                     |
| ------------------------------------ | ------- | ------------------------------------------------------------------------- |
| `ENRICHER_FETCH_CONCURRENCY`         | `15`    | Parallel HTTP fetches per Kafka fetch (IO-bound, can be high)             |
| `ENRICHER_LLM_BATCH_SIZE`            | `5`     | Jobs per single `EnrichBatch` call                                        |
| `ENRICHER_LLM_CALL_INTERVAL_SECONDS` | `5`     | Sleep between successive Stage 2 `EnrichBatch` calls within one fetch (ctx-aware) |
| `ENRICHER_FLUSH_TIMEOUT_SECONDS`     | `120`   | Kafka `FetchMaxWait` — accumulates trickle traffic at the broker          |
| `ENRICHER_DEFAULT_RPS`               | `2`     | Per-adapter token bucket rate (requests/sec) for HTTP fetches             |
| `ENRICHER_DEFAULT_BURST`             | `3`     | Per-adapter token bucket burst capacity                                   |
| `ENRICHER_JITTER_MS`                 | `200`   | Random ±jitter after each rate-limit grant (ms) — anti-bot pacing         |
| `DISCARD_JOB_WITH_EXP_GT`            | `0`     | Drop jobs requiring more than N years exp; 0 = disabled                   |
| `ENRICHER_LLM_SLOTS`                 | `""`    | JSON array of slot configs                                                |

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
