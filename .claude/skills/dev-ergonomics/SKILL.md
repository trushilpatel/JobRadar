---
name: dev-ergonomics
description: Use to audit how ergonomic, consistent, extensible, and agent-legible the codebase (or one component) is to change — measuring friction to change rather than defects. Dispatches dev-ergonomics-auditor agents in parallel per component and synthesizes one ranked recommendation plan. Triggers on "is this codebase ergonomic", "audit dev experience", "where's the friction", "make X easier to extend", "is this consistent/agent-legible". Read-only — recommends, never edits.
---

# Dev Ergonomics

## Overview

Audits **friction to change** — how pleasant, fast, safe, and obvious the codebase is to modify, for the next human _and_ the next AI agent. Dispatches one `dev-ergonomics-auditor` per component in parallel, then synthesizes all reports into a single repo-wide ranked plan with ready-to-paste follow-up prompts.

This skill is **read-only**. It never writes code. It surfaces friction; the human decides what to fix (typically via `feature-team`).

**Source of truth:** `.claude/rules/dev-ergonomics.md` — the ten dimensions and friction-scoring rubric. The auditors score against it; this skill cites it.

**Required sub-skill:** `superpowers:dispatching-parallel-agents` — use it for all agent dispatch.

## What this is NOT (read before dispatching)

- Not correctness / security / test-quality review → that's `quality-team`.
- Not a bug or feature backlog → that's `next-actions`.
- Not a restatement of SOLID, layering, or intrinsic qualities → those live in `backend-solid.md`, `backend-architecture.md`, `quality-attributes.md`.

This measures friction to change. If a finding is really a bug or a feature gap, it gets routed to the right tool, not scored here.

## Step 1 — Establish scope

Read `.claude/rules/dev-ergonomics.md` and the root `CLAUDE.md` first.

**Default: parallel per-component sweep.** Decompose the repo into these components (skip any not present; merge tiny ones):

1. **Backend service slices** — `backend/service/*` (group the small ones; give large slices like `company`, `job` their own auditor).
2. **Backend apps + platform** — `backend/apps/*`, `backend/common/*`, `backend/platform/*`.
3. **Frontend** — `ui/console/src/*` (features, pages, components, lib, config).
4. **Infra** — `infra/`, `k8s/`, `Makefile`, `.husky/`, CI workflows.
5. **Agent tooling** — `.claude/` (rules, skills, agents) + `docs/` + the root `CLAUDE.md` set. The ergonomics of the harness that agents work through is in scope.

If the user names one component ("audit the jobs feature's ergonomics"), default to a **targeted single-component** run — one auditor, full ten dimensions — and skip the sweep. Only sweep the whole repo on an explicit full-repo request.

If the user names a single dimension ("check the whole repo for doc drift"), run a **per-dimension sweep** instead: one auditor per component but with `DIMENSIONS` narrowed to that one.

## Step 2 — Dispatch the auditors

Dispatch **all applicable auditors in one message, in parallel**, via `superpowers:dispatching-parallel-agents`. Each gets:

- `SCOPE` — its component path(s)
- `DIMENSIONS` — all ten (or the narrowed set for a per-dimension run)
- `FOCUS` — pass through any specific concern the user raised (e.g. "consistency across slices", "agent legibility")

Each auditor returns a scorecard + ranked findings + routed items + strongest assets.

## Step 3 — Synthesize

Consolidate every auditor's report into ONE plan. Do not just concatenate — find cross-component patterns (e.g. "three slices each invented their own pagination shape" is one structural finding, not three).

Output:

### 1. Repo ergonomics scorecard

A single table: rows = the ten dimensions, columns = components, cells = grade (A–F). The eye should land instantly on the weakest dimension and the weakest component.

### 2. Ranked action plan — four buckets

Sort every finding into exactly one bucket, P0→P3 within each:

- **Quick wins** — high friction, < 2h fix (rename, delete a dead duplicate, fix a stale CLAUDE.md line).
- **Structural** — consistency / context-locality / scaffolding work that needs a real change (split an 800-line file, unify a divergent pattern, add a template).
- **Doc-drift** — every false-about-current-code claim found (these are hard P0s; agents act on them).
- **Agent-legibility** — friction specific to AI agents (oversized files, missing area `CLAUDE.md`, bypassable gates, three-paths-no-blessed-one).

For each finding: `[P#] title · component · the friction · the recommended change`.

### 3. Top 3 + ready-to-paste prompts

The three highest-leverage items overall, each with a copy-paste `feature-team` prompt to fix it.

### 4. Routed elsewhere

Bugs → `quality-team`; feature gaps → `next-actions`.

Keep it scannable. The reader should know in 30 seconds where the friction is and what to do first.

---

## Self-improvement

After this skill completes, invoke `skill-self-improvement` to evaluate whether this run surfaced anything that should sharpen the audit. Specifically check:

- Did the **dimension model** miss a kind of friction that recurred across components? → propose a targeted edit to `.claude/rules/dev-ergonomics.md`.
- Did a **scoring threshold** (the 800-line trigger, the weights) prove wrong for this codebase? → update the rubric with a dated reason.
- Did the **component decomposition** in Step 1 prove stale (a service area split, a new top-level dir)? → update Step 1.
- Did an auditor consistently produce **false positives** on one check? → add a caveat.
- Was a discovered friction actually a **project-wide invariant** belonging in root `CLAUDE.md`? → propose it there.

Apply the `skill-self-improvement` reproducibility + confidence gate before editing anything. Targeted edits only — never rewrite a section.
