---
title: Claude Coding Standards for JobRadar
date: 2026-04-17
status: approved
---

## Overview

Defines the rules added to `CLAUDE.md` so Claude consistently applies preferred coding standards, workflow, and quality gates in this project without needing to be reminded each session.

## Scope

All changes go into the root `CLAUDE.md`. Existing `.claude/rules/` files (`backend-solid.md`, `backend-architecture.md`, `workflow.md`) remain untouched — the new sections complement them.

## Sections Added

### 1. Implementation Workflow

Before writing any code, Claude must:

- Gather and confirm all requirements; ask for clarification on anything ambiguous
- Map all edge cases before starting implementation
- Only begin coding once edge cases are fully identified

During implementation:

- Proceed autonomously
- Pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences

### 2. Backend Code Standards

Extends the existing SOLID/SOA rules with concrete expectations:

- Structs have one clear reason to change
- Prefer composition; keep interfaces small and focused
- New behaviour via new types/functions — do not modify working code
- Every abstraction must be reusable and extendable without touching existing implementations
- Error handling: wrap with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `infra/logger`
- API responses: always use `Respond[T]`, `RespondPaged[T]`, `RespondError` helpers — never raw `json.NewEncoder` in handlers

### 3. Frontend Code Standards

- Components: small, single-purpose, reusable — one concern per component
- Server state: TanStack Query only — no manual `fetch` + `useState` for API data
- Shared UI state: Zustand — filters, modal state, selections shared across components
- Local state: `useState`/`useReducer` for component-scoped state only
- UI primitives: Radix UI for accessible complex components (dialogs, selects, tooltips)
- Styling: Tailwind classes with `clsx`/`tailwind-merge` — no inline styles; stay consistent with existing design tokens

### 4. Testing and Quality Gates

Every feature delivery includes:

- Tests written alongside the implementation
- All tests run and passing before marking work complete
- Linting passing for both frontend (`eslint`) and backend (`golangci-lint` or equivalent)
- A brief spec-style doc after implementation: file-level references only, no code snippets, not wordy

## Files Affected

| File        | Change                                                                                                     |
| ----------- | ---------------------------------------------------------------------------------------------------------- |
| `CLAUDE.md` | Add four sections: Implementation Workflow, Backend Standards, Frontend Standards, Testing & Quality Gates |
