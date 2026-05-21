# config/ — Application configuration

`config.Load()` tries `.env` in the current working directory first, then `../.env` (repo root), then falls back to environment variables. Called once in each `main.go` before any other setup. All env vars live in a single root `.env`. Full variable reference: [`.env.example`](../../../.env.example).

## Non-obvious config

**LLM slots** — required at startup, two ways to configure (first non-empty wins):

1. YAML file: `LLM_SLOTS_CONFIG=/path/to/llm-slots.yaml` — `${VAR}` syntax expands from env at load time
2. JSON env vars: `FILTER_LLM_SLOTS` / `ENRICHER_LLM_SLOTS` as `[{key_id, api_key, model, rpm, rpd, tpm_in}]`

Current slots file: [`k8s/helm/jobradar/files/llm-slots.yaml`](../../../k8s/helm/jobradar/files/llm-slots.yaml) — single source of truth, loaded directly by Helm via `.Files.Get` and mounted into pods.

**LLM cooldown backoff** — on provider 429: `Base * 2^(n-1)` capped at `Max`, multiplied by `1 ± Jitter`. Success resets count.

| Var                         | Default | Notes                         |
| --------------------------- | ------- | ----------------------------- |
| `LLM_COOLDOWN_BASE_SECONDS` | `60`    | Base backoff                  |
| `LLM_COOLDOWN_MAX_SECONDS`  | `3600`  | Cap                           |
| `LLM_COOLDOWN_JITTER`       | `0.2`   | ± fraction, must be in [0, 1] |

**Enricher two-stage pipeline** — Stage 1 is parallel HTTP fetches, Stage 2 is serial LLM batches. `ENRICHER_FETCH_CONCURRENCY` only affects Stage 1; `ENRICHER_LLM_BATCH_SIZE` only affects Stage 2.

## Usage

```go
cfg := config.Load()
cfg.DB.DSN()               // formatted PostgreSQL DSN
cfg.Kafka.Brokers          // string
cfg.Filter.BatchSize       // int
cfg.Enricher.MaxExpYears   // int (DISCARD_JOB_WITH_EXP_GT)
```
