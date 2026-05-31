# JobRadar Console

React + TypeScript frontend for JobRadar. Displays the job feed, insights, company management, and admin tools.

## Stack

React 19 · TypeScript (strict) · Vite · TanStack Query v5 · TanStack Table v8 · Zustand · Tailwind CSS · Radix UI · Recharts · Supabase Auth · Vitest + Testing Library

## Project layout

```
src/
  api/            ← one file per domain; pure functions, no UI concerns
  types/          ← all shared types in index.ts (single source of truth)
  constants/      ← typed status/stage/reason constants
  lib/            ← api-client, queryClient, supabase, utils (cn, etc.)
  features/       ← one folder per domain: components + hooks + queryKeys + __tests__
  components/
    ui/           ← generic, zero domain knowledge (Button, Badge, etc.)
    shared/       ← lightly domain-aware shared components
  pages/          ← route-level components; compose features only, no data fetching
```

## Local development

```bash
# From repo root
make dev          # Kafka + Redis in Docker; API on :8080; Vite dev server on :5173

# Console only (API must already be running)
make dev-console
```

Auth uses Supabase magic link. Add `http://localhost:5173/auth/callback` to the Supabase allowed redirect URLs.

## Build

```bash
cd frontend && npm run build
```

The Kubernetes image is built via `make k8s-build-console` (from repo root) and served by a static nginx container.

## Tests

```bash
cd frontend && npx vitest run            # all tests
cd frontend && npx vitest run --coverage # with coverage report
cd frontend && npm run lint              # ESLint
cd frontend && npm run tsc               # TypeScript check (no emit)
```

Coverage target is 90% statement coverage per file.

## Key patterns

- **Server state**: TanStack Query only — never `fetch` + `useState` for API data
- **App-wide state**: Zustand (`features/auth/store.ts` for session)
- **HTTP**: always via `lib/api-client.ts` — auth, error handling, envelope unwrapping in one place
- **Query keys**: always imported from `features/<domain>/queryKeys.ts` — never inline strings
- **Components**: Container (fetches, handles loading/error) → Content (pure props, no fetching)
- **Styling**: `cn()` (clsx + tailwind-merge) only — no inline styles
- **Accessible primitives**: Radix UI for dialogs, selects, tooltips, switches
