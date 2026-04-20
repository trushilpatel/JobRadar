# LLM Router Spec (2026-04-19)

- `service/llm/services/LLMRouterService.go` — multi-slot fallback, cooldown integration, exhaustion error
- `service/llm/services/LLMRateLimitService.go` — per-slot reserve + release around inner Scorer
- `service/llm/services/LLMGeminiService.go` — SDK call + 429 detection
- `service/llm/services/LLMFactory.go` — assembles slots → rate-limit → router
- `service/llm/store/LLMRateLimitRedisStore.go` + `reserve.lua` — atomic RPM/RPD/TPM reserve
- `service/llm/store/LLMCooldownRedisStore.go` — exponential backoff with jitter
- `common/redis/client.go` — Redis bootstrap
- `common/config/config.go` — REDIS_*, LLM_COOLDOWN_*, FILTER_LLM_SLOTS, ENRICHER_LLM_SLOTS
- `common/errors/errors.go` — ErrLLM* sentinels + LLMAllExhaustedError
- `docker-compose.yml` — redis service
- `service/llm/CLAUDE.md` — design doc
