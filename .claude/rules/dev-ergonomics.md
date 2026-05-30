---
paths:
  - '.claude/**'
  - 'docs/superpowers/specs/**'
---

# Developer Ergonomics

How pleasant, fast, safe, and **obvious** it is to change this codebase — for the next human _and_ the next AI agent. This is a first-class quality attribute, distinct from whether the software is correct.

## Two audiences, one north star

Every ergonomic decision serves two readers: the next contributor (a stranger, not the author) and the AI agent that does much of the work here. They mostly want the same things — clear boundaries, one obvious path, fresh docs, fast feedback — but agents fail in extra ways: they can't hold a 2000-line file in context, they **act on stale CLAUDE.md as if it were true**, they can't guess the "blessed" path when three exist, and they can't tell a real gate from a polite suggestion. Ergonomics-for-agents is therefore a superset of ergonomics-for-humans.

The north star is the project's first principle: **build for a team, always** (see root `CLAUDE.md`). Friction that one author tolerates silently is a tax the team pays on every change.

## Relation to the other rule files (read this first)

This file measures **friction to change**, not defects. Keep the boundary clean:

- **Intrinsic software qualities** — Reliability, Maintainability, Modularity, Modifiability, Reusability, Testability, **Scalability, Performance** — live in `quality-attributes.md`. That file asks _"is the code good?"_ This file asks _"is it ergonomic to keep it good?"_
- **SOLID** lives in `backend-solid.md`. Single-Responsibility surfaces here as **Context locality (#5)**; Open/Closed surfaces here as **Scaffolding & extensibility (#10)**. We cross-reference SOLID; we don't restate it.
- **Layering** lives in `backend-architecture.md` (backend) and the `features/` / `pages/` / `config/` CLAUDE.md files (frontend).
- **Correctness / security / test-coverage review** is the `quality-team` skill's job. **Bug/feature backlog** is `next-actions`. This file — and the `dev-ergonomics` audit it powers — never duplicates those.

**Readability** and **experiential maintainability** are not separate dimensions here — they are _emergent outcomes_. A codebase reads well exactly when it scores well on **Discoverability (#1) + One blessed path (#3) + Context locality (#5)**. It is pleasant to maintain when it also scores well on **Debuggability (#6) + Safe change (#9)**. If someone asks "where's readability?" — it's those.

---

## The ten dimensions

Each dimension lists the concrete JobRadar signals an audit scores against.

### 1. Discoverability

Can you find where a change goes without reading the whole tree?

- One obvious entry point per concern — a new ATS adapter is a new package + one line in `registry.go`; a new API call is one function in `api/*.ts` + one hook
- An area-level `CLAUDE.md` exists and names the files that matter (the repo's per-directory `CLAUDE.md` convention is the primary discoverability tool — its absence on a non-trivial area is a finding)
- Names describe role, not history (`CompanyAdminListView`, not `CompanyView2`)
- Greppable, structured query keys / log fields / route constants — never free-form strings scattered across files

### 2. Feedback-loop speed

How fast and reliable is edit → test → see-result?

- The inner loop (single-package `go test ./...`, single-feature `vitest`) returns in seconds, not minutes
- Pre-commit stays the floor, not a wall — if it routinely takes minutes, that's friction worth measuring
- **Zero tolerance for flaky tests** — a test that fails intermittently trains everyone to ignore red, the most expensive ergonomic failure there is
- Local-prod parity: k3d is the only deploy target; the inner loop must not require a path that diverges from how prod runs

### 3. One blessed path (consistency)

Is there _one_ way to do a thing, or N divergent ways? Consistency is the single highest-leverage ergonomic and agent-legibility property.

- Exactly one response helper (`handler.Respond[T]` / `RespondPaged[T]`), one HTTP primitive (`apiClient`), one logger, one error-wrapping idiom — used everywhere, no exceptions
- No duplicated helpers that do the same job slightly differently (two date formatters, two pagination shapes)
- A pattern, once chosen, is applied uniformly across every slice — a new slice that invents its own shape is a finding even if it works
- Frontend: server state via TanStack Query only; shared state via Zustand only — no `fetch`+`useState` islands

### 4. Guardrails over guidelines

Is correctness _enforced_, or left to discipline and docs?

- Generated / enforced contracts beat hand-synced duplicates (zod schemas matching backend serialization; FK constraints over convention; CI gates over "I'll remember")
- A rule that matters is a lint rule, a CHECK constraint, a type, or a CI/pre-commit gate — not only a sentence in CLAUDE.md
- Every gate is real: a gate that can be silently bypassed (`--no-verify`, a coverage exclusion that hides real code, a skipped test with no removal condition) is worse than no gate, because it signals false safety
- Typed constants for status/stage/reason — the compiler refuses the wrong value rather than a reviewer catching it

### 5. Context locality

Is everything needed for one logical change co-located, and small enough to hold in one head (or one context window)?

- One file, one reason to change — split when a file fetches _and_ renders _and_ manages state, or a service scans _and_ does analytics
- A single logical change touches few files in one area — "shotgun surgery" across many packages for one feature is a finding
- Large files are a smell _because_ they exceed working memory: a file on a hot change path past ~800 lines is a P0 ergonomic finding (agents cannot reliably edit what they cannot fully load)
- Feature-slice / domain-slice packaging keeps a change's blast radius inside one folder

### 6. Debuggability

When it breaks, how fast can you reach root cause?

- Failures carry context: errors wrapped `fmt.Errorf("ctx: %w", err)` at each layer; sentinel domain errors (`domain.Err*`) instead of string-matching
- Structured logs with field-key constants (`logger.FieldJobID`) + OTel traces propagated through NATS headers — one job is traceable end-to-end
- A bug is cheap to _reproduce_ — deterministic tests (clock injection, in-memory stores), no reliance on real time/network
- The audit trail exists: `job_pipeline_ledger`, `company_scan_log`, `admin_audit_log` — nothing important happens without a record

### 7. Onboarding / time-to-first-change

Clone → first merged change — how long, and how much undocumented tribal knowledge?

- README + Makefile get a fresh contributor (or fresh agent) to a running stack without asking a human
- Documented prerequisites match reality — no "oh, you also need X" surprises (see Doc-truth)
- Setup is a few well-known commands, not a sequence of folklore steps
- The "how do I add one of these?" answer for each unit type (adapter, service, page, migration) is written down or obvious from a template

### 8. Doc-truth (drift)

Do CLAUDE.md / READMEs / specs match reality? **This is the most dangerous dimension for an agent-driven repo** — an agent reads a stale claim and acts on it.

- Every claim in a `CLAUDE.md` about current code is true _today_: named files exist, named tables/columns/flags exist, described flows match the code
- Migration numbers, env var names, and table descriptions in docs track the schema
- Specs in `docs/superpowers/specs/` describe what shipped, not an abandoned plan presented as fact
- **Hard P0:** a `CLAUDE.md` statement that is false about current code — it will actively mislead the next agent

### 9. Safe change / reversibility

Can you change confidently, knowing a regression gets caught?

- The seam you're touching has tests — ≥80% per-package backend coverage is the floor; seam/contract tests guard cross-feature "fix-A-breaks-B" regressions
- Migrations are reversible or explicitly documented one-way; cleanup migrations are ordered after the code that depends on the new state
- Risky change is gated behind a flag with a written ramp/cleanup condition, not a bare edit
- Pipeline changes preserve the "no silent drop" invariant — `RowsAffected` checks return sentinels, never silent nil

### 10. Scaffolding & extensibility (ceremony)

Adding a new unit — generator, template, or copy-paste-and-pray? (This is Open/Closed, felt as friction.)

- Adding a new ATS adapter, service slice, or page follows a known shape with minimal hand-wiring; the ceremony is small and mechanical
- Repeated boilerplate across slices that drifts over time signals a missing template, base type, or codegen step (`BaseService` auto-delegation is the model to extend, not abandon)
- New behaviour arrives via new types/implementations, not by adding optional params to working code or editing every existing slice
- Manual multi-file wiring that's easy to get half-right (forget the `wire_gen.go` line, forget the registry entry) is a candidate for generation or a checklist

---

## Scoring friction (used by the `dev-ergonomics` audit)

Each finding is scored on three axes; the audit ranks across all of them.

**Ergonomic friction score = (frequency × 0.40) + (reach × 0.35) + (fix-cost⁻¹ × 0.25)**

| Axis           | Weight | Score 3                                | Score 2                | Score 1            |
| -------------- | ------ | -------------------------------------- | ---------------------- | ------------------ |
| **Frequency**  | 40%    | Hit on nearly every change in the area | Hit on some changes    | Rare edge          |
| **Reach**      | 35%    | Every contributor _and_ every agent    | A team / one subsystem | One niche workflow |
| **Fix-cost⁻¹** | 25%    | S — under 2h                           | M — half-day           | L — multi-day      |

| Score   | Priority                         |
| ------- | -------------------------------- |
| ≥ 2.5   | **P0** — fix before it compounds |
| 1.8–2.4 | **P1**                           |
| 1.2–1.7 | **P2**                           |
| < 1.2   | **P3**                           |

**Hard P0 overrides (regardless of score):**

- A `CLAUDE.md` / area-doc statement that is **false about current code** (dimension 8)
- A quality gate that can be **silently bypassed** (dimension 4)
- A file on a hot change path **past ~800 lines** (dimension 5)
- An **intermittently flaky** test left in the suite (dimension 2)
