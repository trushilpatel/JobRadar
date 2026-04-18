# Claude Coding Standards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four rule sections to `CLAUDE.md` so Claude consistently applies preferred coding standards, workflow, and quality gates in every session without being reminded.

**Architecture:** All changes are additive edits to the root `CLAUDE.md`. No new files are created. Each section is appended after the existing `## Key rules` section in logical order: workflow first, then backend, then frontend, then quality gates.

**Tech Stack:** Markdown only.

---

## File Map

| File        | Change                                                                                                                                    |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `CLAUDE.md` | Add five sections: Implementation Workflow, Backend Code Standards, Frontend Code Standards, Testing & Quality Gates, Plan Execution Mode |

---

### Task 1: Add Implementation Workflow section

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Add the section after the existing `## Key rules` block**

Append to the end of `CLAUDE.md`:

```markdown
## Implementation workflow

Before writing any code:

- Gather and confirm all requirements; ask for clarification on anything ambiguous
- Map all edge cases before starting implementation
- Only begin coding once edge cases are fully identified and understood

During implementation:

- Proceed autonomously
- Pause and surface the tradeoff when a decision is ambiguous or has meaningful consequences — do not silently pick one
```

- [ ] **Step 2: Verify the section renders correctly**

Open `CLAUDE.md` and confirm the new section appears after `## Key rules` with correct heading level and bullet formatting.

---

### Task 2: Add Backend Code Standards section

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Append backend standards section**

```markdown
## Backend code standards

These extend the rules in `.claude/rules/backend-solid.md` and `.claude/rules/backend-architecture.md`.

- Keep structs small with one clear reason to change
- Prefer composition; keep interfaces small and focused — one interface per concern
- Add new behaviour via new types/functions; do not modify working code to accommodate it
- Every abstraction must be reusable and extendable without touching existing implementations
- Error handling: wrap with `fmt.Errorf("context: %w", err)` at every layer; log only at top-level boundaries via `infra/logger.Logger`
- API responses: always use `Respond[T]`, `RespondPaged[T]`, `RespondError` helpers — never write raw `json.NewEncoder` directly in handlers
```

- [ ] **Step 2: Verify no duplication with existing key rules**

Check that the new rules do not repeat what's already in `## Key rules`. The existing rules cover `infra/logger` and field key constants — the new error handling rule supplements, not duplicates, those.

---

### Task 3: Add Frontend Code Standards section

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Append frontend standards section**

```markdown
## Frontend code standards

**Components**

- Small, single-purpose, reusable — one concern per component
- No god components; split when a component handles more than one responsibility

**State management**

- Server state: TanStack Query only — never `fetch` + `useState` for API data
- Shared UI state (filters, modal open/closed, cross-component selections): Zustand
- Local state (component-scoped only): `useState` / `useReducer`

**UI and styling**

- Use Radix UI primitives for accessible complex components (dialogs, selects, tooltips, switches)
- Use existing libraries before writing custom implementations
- Styling: Tailwind classes composed with `clsx` and `tailwind-merge` — no inline styles
- Stay consistent with existing design tokens and class patterns in the codebase
- Prioritise good UI/UX — accessible, clear, consistent layouts
```

- [ ] **Step 2: Verify stack references match `frontend/package.json`**

Confirm that `@tanstack/react-query`, `@radix-ui/*`, `clsx`, `tailwind-merge`, and `tailwindcss` are all present in `frontend/package.json`. (They are — verified during design.)

---

### Task 4: Add Testing and Quality Gates section

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Append testing and quality gates section**

```markdown
## Testing and quality gates

Every feature delivery must include:

- Tests written alongside implementation — not as an afterthought
- All tests passing before marking work complete
- Linting passing for both frontend (`eslint`) and backend (Go linter)
- Do not mark a task complete if any test or lint check fails — fix the failure first

After implementation, write a brief spec-style doc:

- File-level references only (which file does what) — no code snippets
- Concise — no padding or filler prose
- Saved to `docs/superpowers/specs/` with date prefix
```

- [ ] **Step 2: Confirm lint commands exist**

Check that the project has lint commands available:

- Frontend: `cd frontend && npm run lint`
- Backend: check `Makefile` or `backend/` for a lint target

If commands differ, update the section to use the exact commands found.

---

### Task 5: Add Plan Execution Mode section

**Files:**

- Modify: `CLAUDE.md`

- [ ] **Step 1: Append plan execution mode section**

```markdown
## Plan execution mode

When executing an implementation plan, always use the **subagent-driven approach**:

- Dispatch a fresh subagent per task using the `superpowers:subagent-driven-development` skill
- Review each task's output before dispatching the next
- Never execute plans inline in the same session
```

- [ ] **Step 2: Verify the section is the last section in the file**

Confirm it appears at the end of `CLAUDE.md` with no trailing blank lines after it.
