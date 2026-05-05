
# Core Quality Attributes

These are the non-negotiable qualities every feature must uphold. Each attribute is defined by its concrete expression in this codebase — not abstract theory.

## Reliability
The system performs its intended function correctly, even under adverse conditions.
- Per-job error isolation — one bad Kafka record never stops a worker; errors are logged and routed, not propagated
- Kafka retry ladder (`1m → 10m → 1h → DLQ`) ensures transient failures get multiple chances before giving up
- `job_pipeline_ledger` tracks every job's state — no silent drops anywhere in the pipeline
- SQL `CHECK` constraints + matching Go constants — constrained values enforced at both the DB and application layer

## Availability
The system is operational and accessible when needed.
- Workers are stateless — they can crash and restart without losing data
- Kafka consumers use explicit `CommitRecords` after processing — uncommitted records replay on restart
- `company_scan_queue` heartbeat + `ResetStuck` — crawler crashes are automatically recovered on next scheduler tick
- Non-fatal writes — `job_analytics` snapshot failure never blocks the primary `jobs` write

## Fault Tolerance
The system continues operating despite partial failures.
- Worker loops never exit on a single job failure — catch, route, continue
- Whole-batch LLM failures route each job individually through the retry ladder; no batch is discarded silently
- DLQ is the final safety net — jobs land there for manual inspection, never vanish
- Frontend API errors display user-facing messages; they never crash the UI

## Recoverability
The system can restore itself to a working state after a failure.
- `ResetStuck` in the scheduler reclaims queue rows whose heartbeat expired (crawler crash recovery)
- Uncommitted Kafka offsets replay on restart — no manual intervention needed for transient crashes
- Retry tiers give workers time-based backoff before escalating to DLQ
- `reason='retry'` in `job_pipeline_ledger` is the single source of truth for in-flight retries — never infer from `retry_count > 0`

## Faultlessness
The system operates without defects through layered correctness guarantees.
- Validation layer in the decorator chain catches bad input before it reaches business logic
- Every new feature ships with tests — not as an afterthought
- Type system enforces contracts — narrow interfaces prevent callers from depending on implementation details
- `wire_gen.go` is the only place concrete types are assembled; all other code depends on interfaces

## Maintainability
The system can be understood, modified, and extended without introducing regressions.
- Each file has one clear reason to change — split when a file handles more than one concern
- Decorator chain pattern — extend behaviour by wrapping, not by modifying working code
- Domain errors from `common/domain/` — handlers never inspect raw `error` strings
- All logging through `platform/logger.Logger` with field key constants — never raw strings or `fmt.Println`

## Modularity
The system is composed of independent, well-bounded components.
- Feature-slice packages in `service/` — each domain owns its own models, DTOs, stores, services, and handlers
- ATS adapters are isolated — adding a new adapter is a new package + one line in `registry.go`; nothing else changes
- Pipeline workers are independent processes connected only by Kafka topics and the DB — no direct worker-to-worker calls
- `RestRouter` owns routing; `RestService` owns HTTP handling; neither bleeds into the other

## Reusability
Components are designed to be used in multiple contexts without modification.
- `BaseService` auto-delegates all interface methods — shared pass-through behaviour without duplication
- Platform packages (`logger`, `tracing`, `kafka`, `redis`) are shared across all workers with no coupling to domain logic
- `handler.Respond[T]`, `handler.RespondPaged[T]`, `handler.HandleError` — one place for all HTTP response concerns
- `techtag.Clean` is a pure function — usable anywhere without side effects

## Analyzeability
The system exposes enough information to diagnose problems and understand behaviour.
- OTel traces with W3C propagation through Kafka headers — spans link across service boundaries in Jaeger
- Structured logging with field key constants (`logger.FieldJobID`, `logger.FieldCompany`, etc.) — Loki-queryable
- Per-fetch UUID baked into log context (`fetchLog := log.With(...)`) — trace one job through all workers
- `job_pipeline_ledger` + `company_scan_log` — full audit trail; nothing happens without a record

## Modifiability
The system can be changed safely with minimal risk of introducing defects.
- Open/Closed principle — add behaviour via new types or decorators; never modify working implementations
- `toFetch*Response()` converter pattern in ATS adapters — add a new downstream field by touching only the converter and destination struct, not the HTTP/decode logic
- Interfaces decouple callers from implementations — swap a store, LLM backend, or adapter without touching consumers
- Config via env vars — tune behaviour without redeploying or recompiling

## Testability
Every component can be verified in isolation.
- Narrow interfaces enable lightweight test doubles — no full-stack setup needed for unit tests
- In-memory stores (`LLMRateLimitMemoryStore`, `LLMCooldownMemoryStore`) replace Redis in tests
- Clock injection in workers (`clock` interface) — time-dependent behaviour is deterministic in tests
- No real HTTP in unit tests — use `httptest.NewServer`; no real Kafka — use in-process publisher stubs
- `logger.NewNoop()` silences log output in tests without changing the interface
