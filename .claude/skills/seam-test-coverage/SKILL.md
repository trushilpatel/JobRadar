---
name: seam-test-coverage
description: Use when hardening a frontend page against cross-feature regressions ("fix A breaks B") — adds typed contracts and interaction/contract tests at the seams between features (navigation, shared state, cache, API), enforces zero-`any` typing, and works page-by-page from a living plan so any agent can resume. Triggers on "seam coverage", "stop fix-A-break-B", "harden the jobs/companies page", or continuing Designs/seam-test-coverage-plan.md.
---

# Seam Test Coverage

## Overview

Most "we fixed A and broke B" regressions do not live inside a unit — they live in the **seam between two features**. Per-file unit tests stay green while the integration silently breaks, because each side is correct in isolation and nobody tests the boundary. TypeScript, lint, and unit tests all pass; there is no console error; the page just renders wrong.

This skill closes that gap. For one page at a time, it:

1. Maps the page's seams across four categories.
2. Introduces a **typed contract** at each seam so the wrong wiring becomes unwritable.
3. Adds an **interaction/contract test** that proves the seam transmits.
4. Sweeps the page's slice to **zero `any` / no untyped bindings**.
5. Runs every gate, updates the living plan, commits, and opens a PR.

The work is driven by `Designs/seam-test-coverage-plan.md`, which holds the running status so **any agent can resume** if this one stops or hits a limit.

## When to Use

- Hardening a frontend page/feature against cross-feature regressions.
- Continuing the page-by-page backfill tracked in `Designs/seam-test-coverage-plan.md`.
- After an escaped regression that "passed all tests" — to add the seam test that would have caught it and record the pattern.

Do **not** use for: pure unit-test backfill, backend coverage (use `service-test-lead`), or one-off bug fixes with no cross-feature dimension.

## Resuming / Handoff Protocol

This is a long, multi-PR effort. Treat every run as potentially a handoff.

**To pick up work cold:**

1. Read `Designs/seam-test-coverage-plan.md` top to bottom. The **Status table** is the source of truth.
2. Find the first page whose status is not `Done`. That is your page.
   - `In progress` → read its per-page section; check `git status` / `git log`; continue from the `> RESUME HERE` note.
   - `Not started` → begin the per-page workflow below.
3. Re-read this skill's **Methodology** and **Seam Catalog** — the catalog grows over time.
4. Do exactly one page, then stop at the checkpoint (commit + PR + status update). Do not batch multiple pages into one PR.

**Before you stop (always leave the repo resumable):**

- Update the page's row and per-page section in the plan doc.
- If mid-page and you must stop, write a `> RESUME HERE:` note in that page's section naming the exact next step.
- Never leave the working tree with lint/tsc/tests failing without a note.

## Methodology — 4 seam categories + 1 typing gate

For the current page, audit each category. For every seam found, add the typed contract **and** the test.

### 1. Navigation seams

Any cross-feature `navigate()` / `<Link>` / redirect that carries parameters.

- **Contract:** params must flow through a **typed URL/param builder** (e.g. `jobsUrl()` derived from the same `nuqs` PARSERS the page reads). Never pass cross-page filters via React Router `location.state` unless the destination explicitly reads it.
- **Test shape:** render the source component, trigger the navigation, assert `navigate` was called with a **URL string** whose query carries the param — and assert no stray `state` arg.

### 2. Shared-state seams

State written by one feature and read by another: URL query state (`nuqs`), Zustand stores.

- **Contract:** one exported parser map / store slice is the single source of truth; both writer and reader import it. No inlined query-key strings or store-shape literals.
- **Test shape:** round-trip test between writer and reader, or assert that a builder's output hydrates the consumer's parser.

### 3. Cache seams

Every `useMutation` that, on success, must refresh data owned by another feature.

- **Contract:** `invalidateQueries` keys come from the query-key factory (`queryKeys.ts`), never inlined.
- **Test shape:** assert `invalidateQueries` called with the exact factory keys other features depend on.

### 4. API contracts (fixture-based)

The boundary between a backend response and the frontend's typed, normalized value (`api/*.ts`).

- **Contract:** each `api/*.ts` function returns a **fully-typed, non-null** value; zod schema validates at runtime.
- **Test shape:** feed the function a representative backend payload including `null`/missing fields, assert the output matches the typed shape.

### Typing gate — zero `any`, no untyped bindings

- `no-restricted-imports` boundary rules are **errors** (already enforced).
- Sweep the page's slice for `any`, `as any`, `as unknown as`: replace with explicit named types.
- A page is **not Done** while an `any` remains in its files.

## Per-page workflow

Follow in order. TDD is mandatory for every test (red → green).

1. **Map seams.** Walk the page and its feature slice; list every seam under the 4 categories.
2. **Add typed contracts** where missing (builder, factory key, normalizer return type).
3. **Write tests TDD.** Red first; watch it fail for the right reason; then make it pass.
4. **Sweep `any`.** Grep the page's files; fix with real types.
5. **Gates:**
   ```
   cd ui/console
   npm run lint
   npx tsc --noEmit
   npx vitest run
   npm run build
   ```
6. **Update the plan doc** (status table row + per-page section).
7. **Commit** (no AI attribution; never to main).
8. **`git-push-to-pr` skill** to open the PR.
9. **Checkpoint.** After Jobs page (page 1), stop and let the user confirm the pattern.

## Seam Catalog

A growing registry of real seam-bug patterns. **Append a row whenever a new pattern is found.**

| Pattern                                                           | How it breaks (why units stay green)                                                                                                                                                                                                                                    | Test shape that catches it                                                                                                                                                                                                                                                                                           |
| ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cross-feature nav passes params via React Router `location.state` | Destination reads filters only from the URL (`nuqs`); `state` is silently dropped → page renders unfiltered/empty. No crash, no console error. Source unit test mocks `navigate` and only checks it was called; destination unit test sets filters via URL. Both green. | Interaction test on the source: trigger nav, assert `navigate` called with a **URL string** whose query carries the param, and assert the 2nd arg (`state`) is `undefined`. Plus a typed `jobsUrl()`-style builder as the only sanctioned link. _(Seed: company "View N jobs" → empty Jobs page, fixed 2026-05-28.)_ |

## Quality gates (recap)

- `npm run lint` clean (boundary rules are now errors — no `no-restricted-imports` violations)
- `npx tsc --noEmit` clean
- `npx vitest run` all green (new tests written TDD)
- `npm run build` succeeds
- Plan doc updated; one PR per page; no commit to `main`; no AI attribution

## Common Mistakes

| Mistake                                                               | Fix                                                                  |
| --------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Writing the test after the contract                                   | TDD always — red first, or you never proved the test catches the bug |
| Asserting `navigate` was "called" without checking the argument shape | Assert the actual URL/param payload — that is the seam               |
| Batching several pages into one PR                                    | One page = one PR; keeps the plan resumable and reviews small        |
| Forgetting to update the plan before stopping                         | The plan is the handoff; an un-updated plan strands the next agent   |

## Self-Improvement Footer

After completing a page (or finding a new seam pattern), invoke `skill-self-improvement` to decide whether to update this skill. Specifically:

- **New seam-bug pattern found?** Append a row to the **Seam Catalog**.
- **A category check produced false positives?** Refine that category's description.
- **A project-wide invariant emerged?** Consider `ui/console/CLAUDE.md` per the self-improvement protocol.
