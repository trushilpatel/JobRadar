---
name: feature-team
description: Use when implementing any non-trivial feature end-to-end — coordinates specialist agents through requirements, high-level design, low-level design, test-case design, SWE implementation, and a multi-round QA loop until every gate passes. Triggers on "implement X", "build X", "add X feature", "ship X". Do NOT use for trivial one-liners, doc-only changes, or exploratory spikes.
---

# Feature Implementation Team

## Overview

Six sequential phases take a feature from raw request to production-ready code. No code is written until requirements are locked, both design layers are approved, and every test scenario is mapped. SWE and QA iterate in a closed loop until all feedback is satisfied.

**Required sub-skills:**

- `superpowers:dispatching-parallel-agents` — Phase 5 parallel reviewers
- `superpowers:brainstorming` — used inside Phase 1

---

## Phase Map

```
Phase 1  Requirements Analyst     → HUMAN CHECKPOINT (ambiguities)
Phase 2  High-Level Designer      → HUMAN CHECKPOINT (system design)
Phase 3  Low-Level Designer       → HUMAN CHECKPOINT (file-level design)
Phase 4  Test Case Designer       → test plan locked (no checkpoint unless schema risk)
Phase 5  SWE implements           ← parallel: Security + UX + Observability reviewers
         SWE writes all tests
Phase 6  QA Reviewer verifies
         └── FAIL → SWE fixes → QA re-reviews  (repeat, max 4 rounds)
         └── PASS → present to user
```

---

## Phase 1 — Requirements Analyst

**Runs alone. Blocks everything.**

Use `superpowers:brainstorming` before producing output.

**Reads:** Raw feature request, `CLAUDE.md` (root + subdirs), any referenced existing code or schema.

**Job:** Surface every ambiguity, every edge case, every system the feature touches, and every conflict with existing CLAUDE.md rules before design begins.

**Output — Disambiguation Report:**

```
## Feature: {short name}

## Decisions Needed From Human
1. {question} — Impact: {what changes if answered differently}

## Assumptions (safe defaults if human unavailable)
- {assumption} — Rationale: {why this is the safe default}

## Systems Touched
- Backend: {service domains, workers, DB tables}
- Frontend: {features, pages, components}
- Schema: {new tables / columns / migrations needed}

## Acceptance Criteria
- [ ] {happy path criterion — specific, independently verifiable}
- [ ] {error path criterion}
- [ ] {edge case criterion}
- [ ] {security property}
- [ ] {UX completeness — loading / empty / error states}
- [ ] {observability — what must be logged / traced}

## Out of Scope
- {what this feature explicitly does NOT do}
```

**Human checkpoint:** Present the report. Ask only "Decisions Needed" questions. Wait for answers. Do not proceed until every blocking question is resolved.

**Escalate immediately (do not proceed) if:**

- Decision fundamentally changes which DB tables are used
- Decision changes whether a migration is required
- Decision changes the public API shape
- Request contradicts a CLAUDE.md rule with no clear resolution

---

## Phase 2 — High-Level Designer (HLD)

**Runs alone after Phase 1 is approved. Blocks Phase 3.**

**Reads:** Finalized Disambiguation Report, acceptance criteria, `CLAUDE.md` (root), similar existing features for pattern reference.

**Job:** Design the system shape — what components exist, how they connect, what contracts they expose. No file paths. No method signatures. No implementation details. Only structural decisions.

**Output — High-Level Design Document:**

```
## HLD: {feature name}

## System Components
### New Components
- {ComponentName}: {responsibility in one sentence}
- ...

### Modified Components
- {ComponentName}: {what changes and why}

## Data Flow
{Entry point} → {named transformation steps} → {output}
Error path: {how failures propagate and where they stop}

## API Contract (new or changed endpoints)
| Method | Path | Auth | Rate Limit | Request Shape | Response Shape |
|--------|------|------|-----------|---------------|----------------|

## Data Model
| Table | Column | Type | Constraint | Purpose |
|-------|--------|------|-----------|---------|

### Migration required? {Yes / No}
{If yes: additive only vs. breaking change, estimated impact}

## Key Technology Decisions
- {Decision}: {rationale} (vs. {alternative rejected})

## Risks
- {risk}: {mitigation}

## What the User Sees (if frontend involved)
- New page / section: {description}
- New interaction: {description}
- Error / empty / loading states: {description}
```

**Human checkpoint:** Present the HLD. Flag: new migrations, API contract changes affecting existing clients, any pattern deviation. Wait for explicit approval before Phase 3.

**Escalate immediately if:**

- Design requires a breaking change to an existing public API
- Design requires a pattern not present in the codebase and not covered by CLAUDE.md

---

## Phase 3 — Low-Level Designer / Architect (LLD)

**Runs alone after Phase 2 is approved. Blocks Phase 4.**

**Reads:** Approved HLD, acceptance criteria, actual codebase files for every system touched (decorator chain examples, migration files, existing DTOs, wire files).

**Job:** Design every file, every interface method, every DB column. Be specific enough that the SWE has no open structural questions.

**Output — Low-Level Design Document:**

```
## LLD: {feature name}

## Backend Component Map
| File | Responsibility | Key Interface Methods | Dependencies |
|------|---------------|----------------------|-------------|
| backend/service/{domain}/stores/{Domain}RepositoryService.go | ... | FindByX(ctx, id) (T, error) | GORM |
| backend/service/{domain}/services/{Domain}ImplService.go | ... | ... | Repository |
| backend/service/{domain}/services/{Domain}ValidationService.go | ... | ... | Impl |
| backend/service/{domain}/services/{Domain}RestService.go | ... | ... | Validation |
| backend/service/{domain}/rest/{Domain}RestRouter.go | ... | ... | Rest |

## Frontend Component Map
| File | Responsibility | Key Props / State | Depends On |
|------|---------------|------------------|-----------|
| ui/console/src/features/{name}/pages/{Name}Page.tsx | ... | ... | ... |
| ui/console/src/features/{name}/components/{Name}.tsx | ... | ... | ... |
| ui/console/src/features/{name}/hooks/use{Name}.ts | ... | ... | TanStack Query |
| ui/console/src/api/{name}.ts | ... | ... | api-client |

## DB Schema (exact DDL)
{ALTER TABLE / CREATE TABLE statements or migration SQL}
Migration file: backend/db/migrations/{NNN}_{description}.sql

## Decorator Chain
Repo → Impl → Validation → [Cache?] → Rest → Router
- Repo: {exact DB operations}
- Impl: {business logic, error wrapping}
- Validation: {boundary rules}
- Cache: {present? key pattern, TTL, invalidation trigger}
- Rest: {DTO mapping, handler.Respond calls}

## Ordered Task List (dependency-safe order for SWE)

### Backend (implement in this order)
1. {task} — File: {path} — Interface method(s): {signature}
2. ...

### Frontend (implement in this order)
1. {task} — File: {path}
2. ...

### Shared
1. DB migration — {path} — must run before backend code references new columns
```

**Human checkpoint:** Flag DB migrations and breaking API changes. Wait for explicit approval. Design is locked on approval — no mid-implementation redesigns.

**Escalate immediately if:**

- Design conflict cannot be resolved without changing Phase 2 scope
- A required pattern does not exist in the codebase and CLAUDE.md does not address it

---

## Phase 4 — Test Case Designer

**Runs alone after Phase 3 is approved. Produces the SWE's test mandate.**

**Reads:** Acceptance criteria, HLD, LLD, existing test files in touched packages.

**Job:** Map every test scenario before a single line of implementation is written. The SWE will implement all of these in Phase 5. This is not a checklist — it is the complete test plan.

**Output — Test Plan:**

```
## Test Plan: {feature name}

## Backend Unit Tests (per package)
### Package: {package path}

| Test Name | What It Covers | Input | Expected Outcome |
|-----------|---------------|-------|-----------------|
| TestCreate_HappyPath | ... | valid input | record created, correct response |
| TestCreate_DuplicateKey | ... | duplicate input | domain.ErrConflict returned |
| TestCreate_ValidationReject | ... | missing required field | validation error, no DB call |
| ... | | | |

Table-driven tests (>2 variants):
- TestValidate: {describe variants and what each exercises}

## Backend Integration Tests (real DB / NATS — tag //go:build integration)
| Test Name | What It Covers |
|-----------|---------------|
| ... | |

## Frontend Component Tests
| Component | States to Cover | What to Assert |
|-----------|----------------|---------------|
| {Name}Page | loading, empty, error, success | skeleton renders, empty message, error toast, data displayed |
| {Form}Form | invalid input, submit success, submit error | field errors, success redirect, API error message |
| ... | | |

## Frontend Hook Tests (renderHook)
| Hook | Scenarios |
|------|-----------|
| use{Name} | returns data on success, exposes isLoading, calls invalidate after mutation |

## End-to-End Scenarios (describe flow, not code)
1. {scenario name}: {step-by-step user flow and expected system state at each step}
2. ...

## Edge Cases
- {edge case}: {why it's risky, what the test must assert}
- ...

## Security Test Cases
- {scenario}: {what it verifies — e.g., user A cannot read user B's data}

## Acceptance Criterion Coverage
| Criterion | Covered By |
|-----------|-----------|
| {criterion text} | TestFoo_HappyPath, {Component} success state |
| ... | NOT COVERED — {reason, needs manual test or out of scope} |
```

No human checkpoint needed unless the test plan reveals a new scope question. If it does, surface it before Phase 5.

---

## Phase 5 — SWE + Parallel Reviewers

**Dispatch simultaneously:** SWE, Security Reviewer, UX Reviewer, Observability Reviewer (use `superpowers:dispatching-parallel-agents`).

Reviewers read design docs and implementation as it becomes available. SWE implements and writes **all tests from the Phase 4 Test Plan** before declaring done.

---

### SWE (Software Engineer)

**Reads:** Approved LLD, Phase 4 Test Plan, acceptance criteria, CLAUDE.md (root + subdirs).

**Implementation order:** Follow the LLD Ordered Task List exactly. Backend first (Repo → Impl → Validation → Cache → Rest → Router), then frontend, then integration tests.

**Rules:**

Backend:

- TDD — write the failing test case first, then implement to make it pass
- No GORM outside `RepositoryService`
- No raw `fmt.Errorf` for sentinel errors — use `domain.ServiceError` / `domain.Err*`
- All logging via `platform/logger.Logger` with constants from `platform/logger/fields.go`
- All responses via `handler.Respond[T]`, `handler.RespondPaged[T]`, or `handler.HandleError`
- OTel spans on every public service method

Frontend:

- All HTTP via `lib/api-client.ts` — never raw `fetch`
- All server state via TanStack Query — never `useEffect + useState` for API data
- All className via `cn()` — no inline styles
- No `any` types
- `queryClient.invalidateQueries()` after every mutation
- Loading skeleton, empty state, error state on every data-fetching component

**Tests (written at end of SWE work, from Phase 4 Test Plan):**

- Implement every test case in the Test Plan
- Backend: blackbox tests in `tests/` subpackage, `logger.NewNoop()`, no real DB/NATS in unit tests
- Frontend: `*.test.tsx` co-located, mock at fetch boundary via `vi.fn()`, use `renderHook` for hooks
- Integration tests tagged `//go:build integration`
- Every acceptance criterion must be covered by at least one named test

**Gates (all must pass before SWE declares done):**

- [ ] `go test -race -short ./...` clean
- [ ] `golangci-lint run --config ../.github/.golangci.yml --timeout 5m` zero warnings
- [ ] `scripts/check-coverage.sh /tmp/cov.out 80` passes for every touched package
- [ ] `cd ui/console && npm run lint` zero errors
- [ ] `cd ui/console && npx tsc --noEmit` zero type errors
- [ ] All acceptance criteria covered in Test Plan

**SWE Output Report:**

```
STATUS: DONE | DONE_WITH_CONCERNS | BLOCKED

Files changed:
- {path}: {what changed}

Gate results:
- go test -race: PASS | FAIL
- golangci-lint: PASS | FAIL
- Coverage per package: {package}: {pct}%
- npm lint: PASS | FAIL
- TypeScript: PASS | FAIL

Test Plan coverage:
- [ ] {test name}: IMPLEMENTED
- [ ] {test name}: SKIPPED — {reason}

Assumptions / deviations from LLD:
- {deviation}: {reason}

Concerns (if DONE_WITH_CONCERNS):
- {description}
```

---

### Security Reviewer

**Reads:** HLD, LLD, SWE output files as available.

**Checks:**

1. Every new endpoint: correct middleware (`RequireIdentity` or `RequireRole("admin")`)
2. DB queries: parameterized — no string concatenation with user input
3. Response data: user-scoped — user A cannot read user B's data
4. CORS: no new bypass or overly permissive config
5. Secrets / PII: not present in logs, error messages, or responses
6. Write endpoints: rate-limited
7. Admin endpoints: `RequireRole("admin")` + `AuditAdminMutation`
8. Authorization source: DB (`user_roles`) — never JWT custom claims
9. Validation: at boundary (ValidationService) only — not duplicated deeper
10. Frontend: tokens not logged, auth state not leaked across feature boundaries

**Output: Security Report** — findings tagged `BLOCK` (must fix before QA), `WARN`, `INFO`.

A `BLOCK` finding is reported immediately to the human and halts Phase 6 until resolved.

---

### UX Completeness Reviewer

**Only runs if feature has a frontend component.**

**Reads:** HLD (what the user sees), SWE frontend files.

**Checks:**

1. Every data-fetching component: loading skeleton (not bare spinner)
2. Every list/table: empty state with message or illustration
3. Every query error: user-readable message surfaced (not raw API error string)
4. Every destructive action: confirmation dialog
5. Every form: inline field-level validation messages
6. Interactive elements: keyboard-accessible, correct tab order
7. 429 responses: toast shown using existing project pattern
8. Mobile breakpoints: `sm:` / `md:` Tailwind prefixes on new layouts
9. Shared components: `StatusPill`, `KpiCard`, `DataTable` etc. used where they fit — no duplication

**Output: UX Report** — findings tagged `REQUIRED`, `POLISH`, or `CLEAR` per category.

---

### Observability Reviewer

**Reads:** LLD data flow section, SWE backend files.

**Checks:**

1. Every new public service method: OTel span (`tracing.Start` / `tracing.End`)
2. Error log levels: `logger.Error` for unexpected, `logger.Warn` for recoverable
3. Log fields: constants from `platform/logger/fields.go` — no raw string keys
4. Per-entity log context: baked in at loop start via `log.With(...)`
5. Pipeline events: written to `job_pipeline_ledger` or `company_scan_log`
6. NATS / Kafka: W3C trace headers propagated (`tracing.InjectHeaders` / `tracing.ExtractHeaders`)
7. Worker loops: error isolation — one bad record is caught, logged, skipped; not propagated
8. No `fmt.Println`, `log.Print`, or stdlib `log`

**Output: Observability Report** — findings tagged `REQUIRED`, `RECOMMENDED`, or `CLEAR` per category.

---

## Phase 6 — QA Reviewer (Multi-Round Loop)

**Runs alone after Phase 5 completes. Iterates with SWE until PASS.**

**Inputs every round:** Acceptance criteria, LLD, Phase 4 Test Plan, SWE report, Security report, UX report, Observability report, and any previous QA FAIL issue list.

### QA Checklist — Every Item Verified Explicitly

**Functional**

- [ ] Every acceptance criterion: explicitly confirmed (one line each)
- [ ] Test Plan: every test case implemented or skipped with documented reason
- [ ] Edge cases from Phase 4: exercised by tests
- [ ] End-to-end scenarios: code path traceable from test to assertion

**Backend Gates**

- [ ] All tests passing
- [ ] Coverage ≥80% per touched package (exact numbers)
- [ ] Race detector clean
- [ ] Golangci-lint: zero warnings

**Frontend Gates**

- [ ] All tests passing
- [ ] ESLint: zero errors
- [ ] TypeScript strict: zero errors

**Security**

- [ ] All `BLOCK` findings resolved
- [ ] No `WARN` left without a tracked issue

**UX**

- [ ] All `REQUIRED` findings resolved
- [ ] Loading, empty, error states present for every data-fetching component

**Observability**

- [ ] All `REQUIRED` findings resolved
- [ ] OTel spans on all new service methods
- [ ] Structured logging throughout

**Architecture Compliance**

- [ ] Decorator chain: Repo → Impl → Validation → [Cache] → Rest → Router
- [ ] No GORM outside RepositoryService
- [ ] No `fmt.Errorf` sentinel errors
- [ ] No raw `fetch` in frontend
- [ ] No `useEffect + useState` for server state

### QA Output

**PASS:**

```
VERDICT: PASS  (Round {N})

Confirmed:
- {one-line per criterion}
- All gates: PASS
- Security: no open BLOCKs
- UX: no open REQUIREDs
- Observability: no open REQUIREDs
- Architecture compliance: verified
```

**FAIL:**

```
VERDICT: FAIL  (Round {N})

Issues for SWE:
1. [BACKEND]  {file:line}: {what's wrong and what's required}
2. [FRONTEND] {component}: {what's wrong and what's required}
3. [TEST]     {test name / criterion}: {what's missing}
4. [SECURITY] {issue}: {required fix}
5. [UX]       {component}: {required state/behavior}
6. [OBS]      {file:method}: {required span/log}
7. [DESIGN]   {issue}: {requires Architect re-run — flag to human}

Confirmed clear:
- {items explicitly passed}
```

### Loop Routing

```
iteration += 1

if iteration > 4:
    surface all open issues to human — stop, do not loop further

if FAIL contains [DESIGN]:
    surface to human → re-run Phase 3 (LLD) with issue list → human approves → restart Phase 5

if FAIL contains no [DESIGN]:
    dispatch SWE with issue list → SWE fixes and re-runs gates → re-run Phase 6
```

---

## Human Escalation Points

Stop immediately and surface to the user:

1. **Phase 1 checkpoint** — scope, schema, or API questions that block design
2. **Phase 2 checkpoint** — system design approval (especially migrations)
3. **Phase 3 checkpoint** — file-level design approval before any code is written
4. **Phase 5 — Security BLOCK** — present finding to human before continuing
5. **Phase 5 — SWE BLOCKED** — agent cannot proceed without information or a decision
6. **Phase 6 — iteration > 4** — present all open issues, stop
7. **Phase 6 — [DESIGN] issue** — human must re-approve design before Phase 5 restarts
8. **Phase 5 — existing bug found** — surface it, do not fix silently

---

## What the User Receives at Completion

When QA issues `VERDICT: PASS`:

1. **Feature summary** — 2–4 bullets on what was built
2. **Files changed** — grouped by Backend / Frontend / Tests / Migration
3. **Gate summary** — lint, tests, coverage, TypeScript, race detector
4. **QA rounds** — how many rounds it took
5. **Next step** — invoke `superpowers:finishing-a-development-branch`

Raw agent reports are internal. The user receives the synthesized summary only.

---

## Common Mistakes

| Mistake                                              | Fix                                                                                |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Skipping Phase 1 because requirements "seem clear"   | Phase 1 always runs — it surfaces assumptions, not just questions                  |
| Skipping HLD/LLD split and going straight to code    | Both design layers required; human approves each before proceeding                 |
| Writing tests concurrently with implementation       | Tests are written by SWE at the END of implementation, from the Phase 4 Test Plan  |
| Starting Phase 5 before Phase 4 test plan is done    | Test cases are designed before any code is written                                 |
| Security Reviewer skips frontend output              | Security covers both backend and frontend — always                                 |
| QA issues PASS without checking every checklist item | Every item must be explicitly stated as confirmed                                  |
| Accepting 79% coverage                               | Gate is 80%. Binary. Not negotiable.                                               |
| Routing [DESIGN] issues to SWE                       | DESIGN issues go to Architect — design must be re-locked before Phase 5 restarts   |
| Stopping QA loop after one FAIL                      | Loop continues until PASS or iteration 4 — SWE must address every flagged issue    |
| SWE skips a Test Plan case                           | Every skipped case requires a documented reason; QA will reject undocumented skips |
