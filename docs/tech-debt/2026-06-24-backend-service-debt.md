# Tech Debt Report: backend/service

**Date:** 2026-06-24
**Debt Index:** 70/100

## Scorecard

| Layer          | Dimension           | Grade | Top Finding                                                                                                                              |
| -------------- | ------------------- | ----- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Engineering    | SOLID               | B     | `scan.Service` duplicates all 5 `SchedulerService` methods; LLM store interfaces outside `{Domain}Service.go`                            |
| Engineering    | Loose Coupling      | B     | 13 concrete cross-domain `services/` imports — `application→job`, `crawlernode→job/scan`, `scan→company/job`, `dashboard→scan`           |
| Engineering    | No Magic Values     | C     | Dashboard stage strings duplicate `application/common`; LLM token estimates inline (150/20/500/50); user-admin page limits (20/100) bare |
| Engineering    | Extensibility       | A     | Decorator chain, `BaseService` pattern, and registry-based dispatch all correct                                                          |
| Engineering    | Test-first          | A     | 120 test files, zero skipped tests, zero TODOs                                                                                           |
| Engineering    | Correctness         | B     | `UserAdminRepositoryService` uses `OFFSET` — only remaining instance, explicitly forbidden by `CLAUDE.md`                                |
| Dev-Ergonomics | Discoverability     | D     | 11/12 slices missing area CLAUDE.md; `crawlernode/wire.go` is an empty stub                                                              |
| Dev-Ergonomics | Feedback-loop speed | A     | No flaky tests; no test skips detected                                                                                                   |
| Dev-Ergonomics | One blessed path    | C     | `implLogger` bypasses typed Field API; raw `WriteHeader` in 4 REST services; OFFSET in 2 slices                                          |
| Dev-Ergonomics | Guardrails          | C     | Compile-time interface assertions absent from 5 of 12 slices                                                                             |
| Dev-Ergonomics | Context locality    | B     | `UserJobStateRepositoryService.go` at 831 lines — hard P0                                                                                |
| Dev-Ergonomics | Debuggability       | B     | Raw slog-style string keys in `JobImplService` bypass Loki field constants                                                               |
| Dev-Ergonomics | Onboarding          | C     | No per-slice CLAUDE.md; `crawlernode` wiring undocumented                                                                                |
| Dev-Ergonomics | Doc-truth           | D     | `role/` in CLAUDE.md Packages table does not exist; `insights/wire.go` mislocated vs. layout spec                                        |
| Dev-Ergonomics | Safe change         | C     | `application` `UpdateStage`/`UpdateNote` skip `RowsAffected`; counter delta in `UserJobStateRepositoryService` skips `RowsAffected`      |
| Dev-Ergonomics | Scaffolding         | C     | 3 test placement conventions across 12 slices; `crawlernode/wire.go` empty stub                                                          |

---

## Findings (ranked by debt weight)

### P0 — Fix immediately

**[P0] `role/` package listed in service CLAUDE.md but does not exist** — `backend/service/CLAUDE.md`

- **Dimension:** Doc-truth (hard P0 — false CLAUDE.md claim)
- **Grade:** D → B after fix
- **Debt:** Every agent and contributor reading the Packages table expects `backend/service/role/` to exist. When wiring a permission check they look for it, fail, and may misplace logic or create a duplicate.
- **Fix:** Remove the `role/` row from the CLAUDE.md Packages table until the package is created. If it is planned work, add a note in `docs/superpowers/plans/`.
- **Effort:** S

**[P0] 11 of 12 service slices missing area-level CLAUDE.md** — `backend/service/*/`

- **Dimension:** Discoverability (friction 2.8)
- **Grade:** D → B after fix
- **Debt:** An agent working in `backend/service/company/` gets no file-level orientation, no named entry points, no slice-level invariants (e.g. the dual-surface admin/public split). Must read every file to reconstruct the shape, and may violate invariants documented only at the root service CLAUDE.md (one level up, not auto-loaded).
- **Fix:** Add a one-page CLAUDE.md to each of the 11 slices listing key files, any deviation from the canonical layout, and critical slice-specific rules.
- **Effort:** M

**[P0] `insights/services/wire.go` mislocated vs. canonical layout** — `backend/service/insights/services/wire.go`

- **Dimension:** Doc-truth + Scaffolding (friction 2.6)
- **Grade:** D → B after fix
- **Debt:** The CLAUDE.md layout spec places `wire.go` at the domain root (`insights/wire.go`). It lives at `insights/services/wire.go`. A new slice author following the spec puts their wiring in the wrong place; `llm/` may have the same issue.
- **Fix:** Move `backend/service/insights/services/wire.go` to `backend/service/insights/wire.go` and update any import paths. Verify `llm/` follows the same pattern.
- **Effort:** S

**[P0] `UserJobStateRepositoryService.go` at 831 lines** — `backend/service/job/services/UserJobStateRepositoryService.go`

- **Dimension:** Context locality (hard P0 — hot-path file >800 lines)
- **Grade:** B → A after fix
- **Debt:** File exceeds the 800-line hard P0 threshold. Contains DB queries, counter delta logic, cursor logic, and snapshot helpers in one file. An agent cannot load it in full context alongside test files. Concurrent changes to different user-state concerns have high blast radius.
- **Fix:** Extract the `user_job_counters` counter delta logic (`applyCounterDelta`, `counterDelta`, `counterSnapshot`, `priorSnapshot`, `newSnapshot`) into a separate `UserJobCounterRepositoryService.go` within the same package. The existing interface boundary remains unchanged.
- **Effort:** M

---

### P1 — Fix this sprint

**[P1] 13 concrete cross-domain `services/` package imports** — multiple slices

- **Dimension:** Loose Coupling (EP grade B)
- **Grade:** B → A after fix
- **Debt:** `application→job/services`, `crawlernode→job/services`, `crawlernode→scan/services`, `scan→company/services`, `scan→job/services`, `dashboard→scan/services`. Each import couples a domain to another domain's internal interface declaration. Adding a method to `job.LedgerService` forces edits in `crawlernode`, `scan`, and `application`. The coupling surface grows with each domain connection.
- **Fix:** Each cross-domain consumer should redeclare a narrow local interface (ISP) in its own `{Domain}Service.go`. Example: `crawlernode` needs only `LedgerStore.MarkSeen*` — declare `type LedgerSink interface { ... }` locally and wire the concrete impl in `wire.go`.
- **Effort:** M (per domain; ~4 domains affected)

**[P1] `implLogger` local interface in `JobService.go` bypasses typed Field API** — `backend/service/job/services/JobService.go:227`

- **Dimension:** One blessed path + Debuggability (friction 2.4)
- **Grade:** C → B after fix
- **Debt:** `implLogger` uses slog-style raw string key-value pairs (`"user_id", userID, "err", err.Error()`). The canonical `logger.Logger` interface requires typed `logger.Field` constants from `backend/common/logger/fields.go`. Two logging patterns coexist in the service layer; agents following the job slice's pattern will perpetuate raw string log keys, breaking Loki-queryable field consistency.
- **Fix:** Replace `implLogger` with `logger.Logger`. Convert the `Warn` call to `s.logger.Warn("...", logger.Str(logger.FieldUserID, userID), logger.Err(err))`. Replace the local `noopLogger` struct with `logger.NewNoop()` in tests.
- **Effort:** S

**[P1] OFFSET pagination in `audit/` and `user/admin` contradicts CLAUDE.md rule** — `backend/service/audit/services/AuditRepositoryService.go:66`, `backend/service/user/services/UserAdminRepositoryService.go:53`

- **Dimension:** One blessed path + Doc-truth (friction 2.4)
- **Grade:** C → B after fix
- **Debt:** `CLAUDE.md` states "never use OFFSET-based pagination." Both admin slices break this rule silently. New agents and contributors see the blanket rule, then find two slices that violate it — either the rule is unreliable or these slices need migration.
- **Fix:** Document the admin-only OFFSET exception in `backend/service/CLAUDE.md` with a rationale (admin tables are small; page-jump UX acceptable), or migrate both slices to keyset cursor pagination. The doc exception is S-effort; the migration is L-effort.
- **Effort:** S (doc exception) or L (cursor migration)

**[P1] Compile-time interface assertions absent from 5 slices** — `backend/service/job/`, `application/`, `crawlernode/`, `dashboard/`, `insights/`

- **Dimension:** Guardrails (friction 2.3)
- **Grade:** C → B after fix
- **Debt:** `var _ Interface = (*impl)(nil)` exists in `user/`, `scan/`, `ats/`, `company/`, `audit/` but is absent from 5 slices including `job/` (highest churn) and `crawlernode/` (safety-critical heartbeat logic). A missed method implementation on a new interface method compiles silently and surfaces only at runtime.
- **Fix:** Add one `var _ InterfaceName = (*implType)(nil)` per concrete type in each of the five slices. Minimum: one assertion per `*Store` and `*implService` type in `job/`.
- **Effort:** S

**[P1] `application` `UpdateStage`/`UpdateNote` skip `RowsAffected` check** — `backend/service/application/services/ApplicationRepositoryService.go:150–168`

- **Dimension:** Safe change (friction 2.2)
- **Grade:** C → B after fix
- **Debt:** Both `UpdateStage` and `UpdateNote` execute `db.Exec(UPDATE ... WHERE id = ? AND user_id = ?)` and return `.Error` directly. If the `(id, user_id)` pair does not match (wrong ID, cross-user attempt), the update silently succeeds. Service CLAUDE.md mandates `RowsAffected` checks returning sentinels — this is the "no silent nil" rule.
- **Fix:** Capture `res := db.Exec(...)`, check `res.RowsAffected == 0`, return `domain.ErrNotFound` when zero rows match.
- **Effort:** S

**[P1] `crawlernode/wire.go` is a single empty package declaration** — `backend/service/crawlernode/wire.go`

- **Dimension:** Scaffolding + Discoverability (friction 2.1)
- **Grade:** C → B after fix
- **Debt:** `crawlernode/` has 19 source files and 9 test files — one of the larger slices — but its `wire.go` contains only `package crawlernode`. No `wire.NewSet(...)` or constructor references. Any agent trying to understand how the crawlernode is assembled has no guided entry point.
- **Fix:** Populate `backend/service/crawlernode/wire.go` with a `ProviderSet = wire.NewSet(...)` listing all constructors in dependency order, matching the pattern in `application/wire.go`.
- **Effort:** S

**[P1] Counter delta `applyCounterDelta` skips `RowsAffected` check** — `backend/service/job/services/UserJobStateRepositoryService.go:720`

- **Dimension:** Safe change (friction 1.9)
- **Grade:** C → B after fix
- **Debt:** The `applyCounterDelta` function executes `UPDATE user_job_counters ... WHERE user_id = ?` and checks only `res.Error`. If the counter row doesn't exist (first-time user, migration gap), the delta is silently discarded. The fallback that rebuilds from `GROUP BY` is on reads only — writes silently no-op.
- **Fix:** After `Exec`, check `res.RowsAffected == 0` and either upsert the counter row or return an error for the caller to surface. A `clause.OnConflict{DoUpdate}` seed-then-update pattern avoids the gap.
- **Effort:** M

---

### P2 — Backlog

**[P2] Dashboard stage strings duplicate `application/common` constants** — `backend/service/dashboard/services/DashboardUserImplService.go:83–90`

- **Dimension:** No Magic Values (EP grade C)
- **Grade:** C → B after fix
- **Debt:** Local `displayStageMap` defines raw strings `"applied"`, `"phone_screen"`, `"technical"`, `"final_round"`, `"offer"` identical to `application/common/ApplicationServiceConstants.go`. Adding a new stage requires two manual edits. The comment justifying isolation is incorrect: `common/` packages carry no business-logic coupling.
- **Fix:** Import `appcommon "github.com/trushilpatel/jobradar/service/application/common"` and build the map from `appcommon.Stage*` / `appcommon.DisplayStage*` constants.
- **Effort:** S

**[P2] Inline magic numbers in LLM token-estimate logic** — `backend/service/llm/services/LLMRateLimitService.go:146,154,156`

- **Dimension:** No Magic Values (EP grade C)
- **Grade:** C → B after fix
- **Debt:** `150 + len(jobs)*20`, `total := 500`, `total += 50 + len(in.RawText)/4` — four inline literal magic numbers with no named constants. Tuning token estimates requires hunting scattered literals.
- **Fix:** Promote to named constants: `const (scoreBatchBaseTokens = 150; scoreTokensPerJob = 20; enrichBatchBaseTokens = 500; enrichTokensPerCharFraction = 50)`.
- **Effort:** S

**[P2] Bare page-limit literals in `UserAdminRepositoryService` and `ScanLogRepositoryService`** — `backend/service/user/services/UserAdminRepositoryService.go:45,47`, `backend/service/scan/services/ScanLogRepositoryService.go:116`

- **Dimension:** No Magic Values (EP grade C)
- **Grade:** C → B after fix
- **Debt:** `if limit <= 0 { limit = 20 }`, `if limit > 100 { limit = 100 }`, `Limit(10)` — bare numeric literals with no constants. The `10` constraint appears in both the interface comment and the query body as two separate hand-synced values.
- **Fix:** Declare `const (defaultUserAdminPageLimit = 20; maxUserAdminPageLimit = 100)` in `UserAdminRepositoryService.go`; declare `const recentRunsLimit = 10` in `ScanLogRepositoryService.go`.
- **Effort:** S

**[P2] `scan.Service` interface duplicates all 5 `SchedulerService` methods** — `backend/service/scan/services/ScanService.go:20–67`

- **Dimension:** SOLID ISP
- **Grade:** B → A after fix
- **Debt:** The first 5 methods of `Service` are verbatim copies of `SchedulerService`, acknowledged in a comment. A new scheduler-contract method requires editing both interfaces. `BaseService` auto-delegates all 12 including the 5 duplicated ones.
- **Fix:** Make `Service` embed `SchedulerService` (`type Service interface { SchedulerService; TriggerScan(...) ... }`) and remove the 5 duplicated method declarations.
- **Effort:** S

**[P2] `insights` internal test files diverge from per-domain `tests/` convention** — `backend/service/insights/services/`

- **Dimension:** Discoverability + Scaffolding (friction 1.6)
- **Grade:** C → B after fix
- **Debt:** `repository_internal_test.go` and `velocity_internal_test.go` live inside `services/` (internal tests). Every other domain places tests in a sibling `tests/` directory. Three test placement conventions exist across 12 slices.
- **Fix:** Move `backend/service/crawlernode/services/tests/` to `backend/service/crawlernode/tests/` (matching canonical pattern). Document the internal `_internal_test.go` convention for white-box repository tests in `backend/service/CLAUDE.md`.
- **Effort:** M

**[P2] Raw `w.WriteHeader(http.StatusNoContent)` in 4 REST services** — `user/services/UserRestService.go:96`, `UserAdminRestService.go:122`, `job/services/JobRestService.go:190`, `crawlernode/services/CrawlerNodeRestService.go:75`

- **Dimension:** One blessed path (friction 1.5)
- **Grade:** C → B after fix
- **Debt:** Four services bypass the handler response contract for 204 responses. If OTel response tracing or content-type headers are ever added to `handler.Respond`, these paths are silently excluded.
- **Fix:** Add `handler.RespondNoContent(w http.ResponseWriter)` to the handler package and replace all 4 raw `WriteHeader` calls.
- **Effort:** S

---

### P3 — Nice-to-have

**[P3] LLM store interfaces (`LLMCooldownStore`, `LLMRateLimitStore`) declared in `store/` sub-package** — `backend/service/llm/store/`

- **Dimension:** SOLID (structural rule: interfaces in `{Domain}Service.go` only)
- **Grade:** B — low severity since `store/` is within the same domain
- **Fix:** Move both interface declarations into `backend/service/llm/services/LLMService.go`. Re-export from `store/` only if concrete implementations reference the interface in the same package.
- **Effort:** S

**[P3] `json.Marshal` error silently discarded in 3 cursor-encoding sites** — `job/services/JobRepositoryService.go:52`, `company/services/CompanyAdminRepositoryService.go:31`, `CompanyRepositoryService.go:41`

- **Dimension:** Debuggability (friction 1.4) — technically safe (primitive-only structs), but sets a copy-paste precedent
- **Fix:** Add comment `// Marshal of primitive-only structs is infallible — error safely ignored` at each `_ =` discard to make intent explicit.
- **Effort:** S

---

## Proposed Fix Order

Ready-to-paste prompts, ordered P0 → quick-wins (S effort) → structural (L effort):

1. `/feature-team remove the non-existent role/ row from backend/service/CLAUDE.md Packages table` — P0 doc-truth, S
2. `/feature-team move backend/service/insights/services/wire.go to backend/service/insights/wire.go and update all import paths; verify llm/ follows the same pattern` — P0 doc-truth, S
3. `/feature-team add compile-time interface assertions (var _ Interface = (*impl)(nil)) to backend/service/job/, application/, crawlernode/, dashboard/, insights/ — one assertion per concrete type` — P1 guardrails, S
4. `/feature-team populate backend/service/crawlernode/wire.go with ProviderSet = wire.NewSet(...) listing all crawlernode constructors in dependency order` — P1 scaffolding, S
5. `/feature-team fix application UpdateStage and UpdateNote in ApplicationRepositoryService.go to check RowsAffected and return domain.ErrNotFound on zero rows` — P1 safe-change, S
6. `/feature-team replace the implLogger interface in backend/service/job/services/JobService.go with logger.Logger and convert all Warn calls to use logger.Field constants` — P1 one-blessed-path, S
7. `/feature-team extract LLM token-estimate magic numbers in LLMRateLimitService.go into named constants (scoreBatchBaseTokens, scoreTokensPerJob, enrichBatchBaseTokens)` — P2 no-magic-values, S
8. `/feature-team replace raw w.WriteHeader(http.StatusNoContent) in UserRestService.go, UserAdminRestService.go, JobRestService.go, CrawlerNodeRestService.go with a new handler.RespondNoContent() helper` — P2 one-blessed-path, S
9. `/feature-team fix dashboard DashboardUserImplService.go to import application/common constants instead of redeclaring stage strings locally` — P2 no-magic-values, S
10. `/feature-team add named constants for page limits in UserAdminRepositoryService.go (defaultUserAdminPageLimit=20, maxUserAdminPageLimit=100) and recentRunsLimit=10 in ScanLogRepositoryService.go` — P2 no-magic-values, S
11. `/feature-team make scan.Service embed SchedulerService in ScanService.go instead of duplicating the 5 scheduler method declarations` — P2 SOLID, S
12. `/feature-team extract UserJobCounterRepositoryService.go from the counter delta logic in UserJobStateRepositoryService.go (lines ~720+) to bring the file under 800 lines` — P0 context-locality, M
13. `/feature-team fix applyCounterDelta in UserJobStateRepositoryService.go to check RowsAffected and upsert the counter row when missing` — P1 safe-change, M
14. `/feature-team add per-slice CLAUDE.md files to all 11 missing service slices (application, ats, audit, company, crawlernode, dashboard, insights, job, scan, testutil, user)` — P0 discoverability, M
15. `/feature-team declare narrow local cross-domain interfaces in crawlernode (LedgerSink), application, scan, and dashboard instead of importing foreign services/ packages directly` — P1 loose-coupling, M

---

## Assets — What's Working

1. **Decorator chain compliance is exemplary.** All 12 slices follow `RestRouter → RestService → Validation → Impl → Repository` correctly. `BaseService` auto-delegation is uniformly applied. Zero business logic leaks into wiring-layer files.
2. **Test discipline is exceptional.** 120 test files, zero skipped tests, zero TODOs/FIXMEs, zero flaky markers. The codebase is unusually clean here and must be protected.
3. **Extensibility architecture is solid.** Adding a new service domain is additive-only: one new directory, one line in `wire.go`. The `BaseService` pattern eliminates boilerplate delegation.
4. **`RepositoryService` terminal pattern is respected everywhere.** All repository files are correctly terminal: no `next` field, no `BaseService` embedding, all checked by comment and code.
