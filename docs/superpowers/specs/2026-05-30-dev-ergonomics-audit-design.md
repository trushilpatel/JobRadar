# Dev-Ergonomics Audit System

**Date:** 2026-05-30

## Problem

The project measures whether the software is _good_ (`.claude/rules/quality-attributes.md`, `quality-team` skill) but has nothing that measures whether the codebase is _good to change_ — friction to change, consistency, extensibility, and AI-agent legibility. In an agent-driven repo, that meta-layer is a first-class quality attribute and was unowned.

## What shipped

A read-only, advisory audit system — three new artifacts plus two gap fixes — that recommends ergonomic changes; it never edits code (fixes route to `feature-team`).

### Canonical model

- `.claude/rules/dev-ergonomics.md` — ten ergonomics dimensions (Discoverability, Feedback-loop speed, One blessed path/consistency, Guardrails over guidelines, Context locality, Debuggability, Onboarding, Doc-truth/drift, Safe change, Scaffolding/extensibility), each with concrete JobRadar signals; plus the friction-scoring rubric (frequency/reach/fix-cost) and hard-P0 overrides. Opens with a boundary note: Readability and experiential-Maintainability are emergent outcomes of dimensions 1/3/5/6/9; cross-references `backend-solid.md`, `backend-architecture.md`, `quality-attributes.md`. Auto-loads on `.claude/**` and `docs/superpowers/specs/**`.

### Worker agent

- `.claude/agents/dev-ergonomics-auditor.md` — audits one component against the ten dimensions, gathers cited evidence, scores and ranks findings, routes non-ergonomic findings to `quality-team`/`next-actions`, names strongest assets. Read-only.

### Orchestrator skill

- `.claude/skills/dev-ergonomics/SKILL.md` — establishes scope (default: parallel per-component sweep — backend slices, apps+platform, frontend, infra, agent-tooling; targeted single-component or per-dimension on request), dispatches auditors in parallel via `superpowers:dispatching-parallel-agents`, synthesizes one repo-wide scorecard + four-bucket ranked plan (Quick wins / Structural / Doc-drift / Agent-legibility) with ready-to-paste follow-up prompts. Carries a `skill-self-improvement` footer.

### Gap fixes (quality model completeness)

- `.claude/rules/quality-attributes.md` — added Scalability and Performance Efficiency sections.
- `docs/QUALITY.md` — count 11→13, added the two attribute tables, added the ergonomics cross-reference.
- `CLAUDE.md` — "See also" now points at both `quality-attributes.md` (is the code good?) and `dev-ergonomics.md` (is it ergonomic to keep it good?).

## Separation of concerns

`dev-ergonomics` measures friction to change only. Correctness/security/test-quality stays with `quality-team`; bug/feature backlog stays with `next-actions`; SOLID/layering/intrinsic qualities stay in their existing rule files. Both the rule file, the agent, and the skill state this boundary and route misfiled findings rather than absorbing them.
