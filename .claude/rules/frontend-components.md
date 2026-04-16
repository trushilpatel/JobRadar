---
paths:
  - 'frontend/**/*.ts'
  - 'frontend/**/*.tsx'
---

# Frontend — Component Architecture & Best Practices

## Component decomposition

Split a UI piece into its own component when any of these are true:

- It appears in more than one place
- It has its own state or data-fetching logic
- It is complex enough that the parent becomes hard to read without it
- It maps to a distinct user concept (a card, a panel, a form, a tab)

Don't split just for splitting's sake — a tiny presentational snippet that's only used once can stay inline.

## Component structure conventions

- **Pages** (`src/pages/`) — route-level components, compose features, minimal logic
- **Features** (`src/features/<domain>/`) — domain-specific components, hooks, and helpers co-located
- **Shared / UI** (`src/components/`) — fully generic, reusable, no domain knowledge

## Hooks

- Extract any non-trivial state or side-effect logic into a custom hook (`use*.ts`)
- A component should read from hooks and render — not contain raw `useEffect`/`useState` chains
- Keep hooks focused: one hook per concern

## Use the available libraries — don't reinvent

- Use existing UI component library (check what's already in `package.json` before building a custom component)
- Use React Query / SWR patterns already established in the codebase for data fetching
- Use existing utility functions — check `src/utils/` before writing a helper
- Reach for a well-known library before implementing common patterns (date formatting, validation, etc.)

## General best practices

- Props interfaces should be explicit and typed — no `any`
- Avoid prop drilling beyond 2 levels — lift state or use context/store
- Side effects in `useEffect` should have correct dependency arrays
- Prefer controlled components for forms
- Keep components pure where possible — same props → same output
