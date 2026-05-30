# JobRadar — Core Quality Attributes

These 13 attributes are the non-negotiable qualities every feature must uphold, expressed through concrete patterns in this codebase. Detailed rules auto-load via `.claude/rules/quality-attributes.md` (backend) and `.claude/rules/frontend-quality-attributes.md` (frontend).

These attributes ask _"is the code good?"_ The complementary _"is it ergonomic to keep it good?"_ — friction to change, consistency, extensibility, agent-legibility — is the **developer-ergonomics** model in `.claude/rules/dev-ergonomics.md`, audited by the `dev-ergonomics` skill.

---

## Reliability

_The system performs its intended function correctly, even under adverse conditions._

| Layer    | How it's enforced                                                                                                                                    |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | Per-job error isolation — one bad Kafka record never stops a worker                                                                                  |
| Backend  | `job_pipeline_ledger` tracks every job — no silent drops anywhere in the pipeline                                                                    |
| Backend  | SQL `CHECK` constraints + matching Go constants — constrained values enforced at both layers                                                         |
| Backend  | Cache reads via `backend/platform/cache.Cache` (Redis) with `GetOrLoad[T]` single-flight — concurrent misses collapse into one DB load, no stampedes |
| Frontend | All HTTP through `lib/api-client.ts` — auth, error handling, envelope unwrapping in one place                                                        |
| Frontend | All domain types in `types/index.ts` — one source of truth, no duplicated definitions                                                                |

---

## Availability

_The system is operational and accessible when needed._

| Layer    | How it's enforced                                                                                                                  |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | Workers are stateless — crash and restart without data loss                                                                        |
| Backend  | Explicit `CommitRecords` after processing — uncommitted records replay on restart                                                  |
| Backend  | `company_scan_queue` heartbeat + `ResetStuck` — crawler crashes auto-recovered on next scheduler tick                              |
| Backend  | Per-user rate limit (`backend/platform/ratelimit`) **fails open** on Redis outage — limiter errors don't reject legitimate traffic |
| Frontend | TanStack Query `staleTime: 30s` + `gcTime: 5m` — cached data keeps the UI functional during network hiccups                        |
| Frontend | Auth token auto-refreshed on 401 with deduplication — users never see an unexpected logout                                         |

---

## Fault Tolerance

_The system continues operating despite partial failures._

| Layer    | How it's enforced                                                              |
| -------- | ------------------------------------------------------------------------------ |
| Backend  | Worker loops never exit on a single job failure — catch, route, continue       |
| Backend  | Whole-batch LLM failures route each job individually through the retry ladder  |
| Backend  | `job_analytics` snapshot failure is non-fatal — `jobs` write already succeeded |
| Frontend | TanStack Query `retry: 1` — transient failures get one automatic retry         |
| Frontend | `ApiValidationError` shows field-level errors inline — never crash screens     |

---

## Recoverability

_The system can restore itself to a working state after a failure._

| Layer    | How it's enforced                                                                       |
| -------- | --------------------------------------------------------------------------------------- |
| Backend  | `ResetStuck` in scheduler reclaims expired heartbeat rows (crawler crash recovery)      |
| Backend  | Uncommitted Kafka offsets replay on restart — no manual intervention needed             |
| Backend  | Retry tiers (`1m → 10m → 1h → DLQ`) give time-based backoff before escalating           |
| Frontend | `queryClient.invalidateQueries()` after every mutation — data refetched immediately     |
| Frontend | `onAuthStateChange` listener restores session after page refresh — no re-login required |

---

## Faultlessness

_The system operates without defects through layered correctness guarantees._

| Layer    | How it's enforced                                                                                                                                              |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | Validation layer in the decorator chain catches bad input before business logic                                                                                |
| Backend  | Every feature ships with tests — not as an afterthought                                                                                                        |
| Backend  | Type system enforces contracts — narrow interfaces prevent depending on implementation details                                                                 |
| Backend  | User-scoped repos take `userID` explicit; `service/testutil.AssertUserScoped` (Phase 1, consumed by Phase 2 repos) guards every method against an empty userID |
| Frontend | No `any` types — all props, API responses, and state explicitly typed                                                                                          |
| Frontend | Static CONFIG maps for display variants — no switch statements, no missed cases                                                                                |
| Frontend | `cn()` for all className composition — no conflicting Tailwind classes                                                                                         |

---

## Maintainability

_The system can be understood, modified, and extended without introducing regressions._

| Layer    | How it's enforced                                                                                                                                                              |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Backend  | Each file has one clear reason to change — split when a file handles more than one concern                                                                                     |
| Backend  | Decorator chain — extend behaviour by wrapping, never modifying working code                                                                                                   |
| Backend  | All logging through `platform/logger.Logger` with field key constants — never raw strings                                                                                      |
| Backend  | Identity flows via `context.Context` (`backend/common/identity`) — services pull `userID` from ctx, repos take it as an explicit parameter; one place to change the auth model |
| Frontend | Feature-slice structure (`features/auth/`, `features/jobs/`, etc.) — change one feature without touching others                                                                |
| Frontend | `lib/queryClient.ts` holds TanStack Query defaults — tune globally in one place                                                                                                |

---

## Modularity

_The system is composed of independent, well-bounded components._

| Layer    | How it's enforced                                                                                                                     |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | Feature-slice packages in `service/` — each domain owns models, DTOs, stores, services, handlers                                      |
| Backend  | ATS adapters are isolated — new adapter = new package + one line in `registry.go`                                                     |
| Backend  | Pipeline workers are independent processes — connected only by Kafka and the DB                                                       |
| Backend  | Identity decoupled from transport — `backend/common/identity` is consumed by middleware, services, and tests via the same context API |
| Frontend | Presentational components (`StatusPill`, `KpiCard`) carry no domain knowledge                                                         |
| Frontend | API functions in `api/*.ts` are pure — no UI concerns, no side effects                                                                |

---

## Reusability

_Components are designed to be used in multiple contexts without modification._

| Layer    | How it's enforced                                                                      |
| -------- | -------------------------------------------------------------------------------------- |
| Backend  | `BaseService` auto-delegates — shared pass-through without duplication                 |
| Backend  | Platform packages (`logger`, `tracing`, `kafka`, `redis`) shared across all workers    |
| Backend  | `handler.Respond[T]`, `handler.HandleError` — one place for all HTTP response concerns |
| Frontend | `components/ui/` and `components/shared/` — use before writing a new component         |
| Frontend | Custom hooks over raw `useQuery` — extract when two components need the same query     |

---

## Analyzeability

_The system exposes enough information to diagnose problems and understand behaviour._

| Layer    | How it's enforced                                                                                                                            |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | OTel traces with W3C propagation through Kafka headers — spans link across service boundaries                                                |
| Backend  | Structured logging with field key constants — Loki-queryable by `job_id`, `company_id`, etc.                                                 |
| Backend  | Per-fetch UUID baked into log context — trace one job through all workers                                                                    |
| Backend  | `job_pipeline_ledger` + `company_scan_log` — full audit trail; nothing happens without a record                                              |
| Backend  | `admin_audit_log` (Phase 1) — every admin mutation appends an entry via `AuditAdminMutation` middleware; full forensic trail of who did what |
| Frontend | Query keys as structured string arrays (`['company', id]`) — greppable, precisely invalidatable                                              |
| Frontend | Zustand auth store is the single source of session truth — one place to inspect                                                              |

---

## Modifiability

_The system can be changed safely with minimal risk of introducing defects._

| Layer    | How it's enforced                                                                             |
| -------- | --------------------------------------------------------------------------------------------- |
| Backend  | Open/Closed — add behaviour via new types or decorators; never modify working implementations |
| Backend  | `toFetch*Response()` converter pattern — add a new field by touching only the converter       |
| Backend  | Config via env vars — tune behaviour without redeploying or recompiling                       |
| Frontend | New feature = new folder under `features/` — no existing feature is touched                   |
| Frontend | API shape changes isolated to `api/*.ts` — consuming hooks and components unchanged           |

---

## Testability

_Every component can be verified in isolation._

| Layer    | How it's enforced                                                                      |
| -------- | -------------------------------------------------------------------------------------- |
| Backend  | Narrow interfaces enable lightweight test doubles — no full-stack setup for unit tests |
| Backend  | In-memory stores replace Redis in tests (`LLMRateLimitMemoryStore`, etc.)              |
| Backend  | Clock injection in workers — time-dependent behaviour is deterministic in tests        |
| Backend  | No real HTTP in unit tests — `httptest.NewServer`; no real Kafka — in-process stubs    |
| Frontend | `apiClient` mockable via `vi.fn()` — tests replace `fetch`, not the full HTTP stack    |
| Frontend | Test-specific `queryClient` — no cache state leaks between tests                       |
| Frontend | Hooks testable via `renderHook` — components stay thin and independently verifiable    |

---

## Scalability

_The system handles growth — more companies, jobs, users — without redesign._

| Layer   | How it's enforced                                                                                                  |
| ------- | ------------------------------------------------------------------------------------------------------------------ |
| Backend | Workers are stateless and horizontally scalable — coordination via the DB queue and NATS, never in-process state   |
| Backend | `company_scan_queue` is the unit of fan-out — work sharded by claiming rows, not by partitioning code              |
| Backend | Keyset (cursor) pagination on every list endpoint — query cost stays constant as tables grow                       |
| Backend | LLM throughput scales via explicit rate-limit + cooldown slots — back-pressure, never unbounded fan-out            |
| Backend | Per-user overlays (`user_job_state`) keep the global `jobs` feed shared — user growth doesn't multiply the dataset |

---

## Performance Efficiency

_The system uses time, memory, and external quota deliberately under fixed resource limits._

| Layer   | How it's enforced                                                                                                 |
| ------- | ----------------------------------------------------------------------------------------------------------------- |
| Backend | Resource limits are intentional (128Mi k3d) — code runs within them; never assume headroom                        |
| Backend | Hot queries are index-backed (`uq_jobs_job_post_id`, queue/ledger partial indexes) — O(log n), not scans          |
| Backend | NATS JetStream decouples producers from consumers — a slow analyzer never blocks the crawler                      |
| Backend | LLM calls batched and quota-aware — the costliest dependency spent deliberately, with retry ladders not hot loops |
| Backend | Non-fatal `job_analytics` writes never sit on the critical path of the primary `jobs` write                       |
