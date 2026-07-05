# service/llm/ — LLM routing + rate limiting + cooldown

## Interfaces (`services/LLMService.go`)

- **`JobsRelevanceFilter`** — batch-scores job summaries, returns IDs of relevant jobs. Used by the filter worker.
- **`JobPostEnricher`** — extracts structured data from a full job posting. Used by the analyzer worker.
- **`ResumeKeywordExtractor`** — extracts technology/skill keywords from a resume's plain text body (`dto.ResumeKeywordInput` → `dto.ResumeKeywordResult`). Used by `service/resume` (best-effort after upload, and the explicit `POST /resumes/{id}/keywords` retry) via a narrow same-shaped interface declared in `service/resume/services/ResumeService.go` — the resume domain never depends on the full `Scorer` contract.
- **`Scorer`** — combines all three. Returned by `NewScorer`; callers receive the narrower interface they need.

Adding a method to `Scorer` requires implementing it on both decorators (`geminiScorer`, `rateLimitService`, `routerService`) — the router/rate-limit chain is mechanical per-method wrapping, not automatic delegation, so a new interface method is NOT free; it is, however, the _only_ place new plumbing is needed — no new decorator types, no new rate-limit/cooldown store shape.

## Decorator chain

```
Router → per-slot RateLimit → GeminiService (per-slot)
          ↘ CooldownStore (injected)
          ↘ RateLimitStore (injected)
```

- Every configured `(provider, keyID, model)` becomes a **slot**. Each slot holds its own `geminiScorer` + `LLMRateLimitService`.
- The **router** picks the first non-cooled slot. On `ErrLLMLocalQuotaExceeded` it skips without touching cooldown. On `ErrLLMProviderRateLimited` it records a cooldown failure and tries the next slot. Other errors bubble. When no slot succeeds it returns `*LLMAllExhaustedError`.

## Layout

```
service/llm/
  common/          ← provider + outcome constants, SlotID()
  dto/             ← JobsRelevanceFilterInput, JobPostEnrichInput, JobPostEnrichResult, SlotStatus
  prompts/         ← BuildScoreBatch, BuildEnrich
  services/
    LLMService.go           ← interfaces (contract)
    LLMGeminiService.go     ← per-slot Gemini client; maps 429 → ErrLLMProviderRateLimited
    LLMRateLimitService.go  ← per-slot decorator over Scorer
    LLMRouterService.go     ← multi-slot router; terminal Scorer
    LLMFactory.go           ← Options → router
  store/
    LLMRateLimitStore.go        ← interface + ReserveResult
    LLMRateLimitRedisStore.go   ← Redis + Lua reserve.lua
    LLMRateLimitMemoryStore.go  ← tests
    LLMCooldownStore.go         ← interface
    LLMCooldownRedisStore.go    ← Redis impl
    LLMCooldownMemoryStore.go   ← tests
    reserve.lua                 ← atomic RPM + RPD + TPM reserve
```

## Rate-limit store

Three dimensions per slot, all `0 = unlimited`:

| Dim   | Shape                                             |
| ----- | ------------------------------------------------- |
| RPM   | sliding 60s ZSET (member=reqID, score=now_millis) |
| RPD   | fixed UTC-day INT counter (`…:day:{YYYY-MM-DD}`)  |
| TPMIn | sliding 60s ZSET + HASH(reqID→tokens) + INT(sum)  |

Reservation is atomic via `reserve.lua`. Release rolls back a reservation (non-429 inner failures).

## Cooldown store

Per-slot exponential backoff with jitter. Keys: `cooldown:{slotID}:failure_count`, `cooldown:{slotID}:cooldown_until` (unix millis). Policy: `Base * 2^(n-1)` capped at `Max`, multiplied by `1 ± Jitter`. Success → count reset + until deleted.

## Errors (`common/errors`)

| Sentinel                    | Meaning                             | Router reaction   |
| --------------------------- | ----------------------------------- | ----------------- |
| `ErrLLMValidation`          | bad input                           | caller bug (4xx)  |
| `ErrLLMLocalQuotaExceeded`  | our own RPM/RPD/TPM counter said no | skip, no cooldown |
| `ErrLLMProviderRateLimited` | provider 429                        | record cooldown   |
| `ErrLLMAllExhausted`        | all slots cooled or over quota      | fail-fast (503)   |

`*LLMAllExhaustedError` carries `[]LLMSlotStatus` + `RetryAfter` for the caller.

## Factory

- `NewScorer(Options)` **requires** `RateLimitStore` and `CooldownStore`.
- `Slots[]` preferred. Legacy `APIKey` + `Model` + `Limits` kept for back-compat — becomes a single `default` slot.
- Duplicate `(provider, keyID, model)` → error at startup.

## Rules

- Slot decisions belong in the router — never in workers
- `geminiScorer` stays focused: SDK call + JSON parse + 429 detection. No rate limiting inside
- All Redis access goes through `store/` — no `*redis.Client` outside that subpackage
- `ErrLLMLocalQuotaExceeded` **never** triggers cooldown — it's a routine skip
- Only real `ErrLLMProviderRateLimited` increments `failure_count`
- Workers depend on `JobsRelevanceFilter` / `JobPostEnricher` — never on `*geminiScorer` directly
