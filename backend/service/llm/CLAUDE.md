# service/llm/ — LLM evaluation service

## Interfaces (`services/LLMService.go`)

**`JobsRelevanceFilter`** — batch-scores a list of minimal job summaries, returns IDs of relevant jobs. Used by the filter worker.

**`JobPostEnricher`** — extracts structured data from a single full job posting. Used by the analyzer worker.

**`Scorer`** — combines both interfaces. Returned by `NewScorer`; callers receive the narrower interface they need.

## Structure

```
service/llm/
  common/     ← constants: provider names, max output tokens
  dto/        ← JobsRelevanceFilterInput, JobPostEnrichInput, JobPostEnrichResult
  prompts/    ← BuildScoreBatchPrompt, BuildEnrichPrompt — one file per prompt
  services/   ← LLMService.go (interfaces), LLMGeminiService.go, LLMImplService.go, LLMFactory.go
```

## Prompt design rule

- **Fixed** (hardcoded in `prompts/`): output format, exclusion rules, response structure
- **Dynamic** (injected via `Options`): `TargetRoles`, `YearsExp` — never hardcoded

## Factory

`NewScorer(Options)` builds Gemini → wraps with rate limiter if limits are set. Adding a new provider means adding one `case` in `LLMFactory.go`.

## Rate limits (`LLMImplService.go`)

`RateLimits` enforces three quota dimensions — all optional (0 = unlimited):

| Field | Meaning |
|-------|---------|
| `RPM` | requests per minute (token bucket) |
| `RPD` | requests per day (hard counter, resets UTC midnight) |
| `TPMIn` | input tokens per minute (token bucket) |

## Rules

- Workers depend on `JobsRelevanceFilter` or `JobPostEnricher` — never on `*geminiScorer` directly
- Job eligibility dropping (`dropJob`) lives in the analyzer worker — not here
- Batch errors in filter mark all jobs `reason='error'` in ledger — never re-queue
