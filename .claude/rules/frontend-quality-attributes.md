---
paths:
  - 'ui/console/**/*.tsx'
  - 'ui/console/**/*.ts'
---

# Frontend Core Quality Attributes

These define the non-negotiable qualities every frontend feature must uphold. Each attribute is expressed through its concrete pattern in this codebase — not abstract theory.

## Reliability
The UI consistently reflects correct state and recovers from API failures without breaking.
- All HTTP requests go through `lib/api-client.ts` — auth, error handling, and envelope unwrapping in one place; never call `fetch` directly
- Response envelopes (`{data}`, `{data, total, page, limit}`) are unwrapped in `apiClient` — components receive clean typed values, never raw envelopes
- All domain types live in `types/index.ts` — one source of truth; no duplicated or inlined type definitions
- Static CONFIG maps for display mappings (e.g. `StatusPill`) — no switch statements, no missed cases

## Availability
The UI is always in a defined, usable state — never blank or broken during loading or network issues.
- TanStack Query `staleTime: 30s` + `gcTime: 5m` — cached data keeps the UI functional during network hiccups
- Show loading skeletons while queries are pending — never blank content where data should appear
- Auth token is auto-refreshed on 401 with deduplication — concurrent requests don't spawn multiple refresh calls; users never see an unexpected logout
- `refetchInterval` on live data panels — data self-heals without user action

## Fault Tolerance
The UI degrades gracefully when APIs fail; one failure never breaks unrelated features.
- TanStack Query `retry: 1` — transient network failures get one automatic retry before surfacing an error state
- API errors are parsed via `lib/api-error.ts` — field-level validation errors show inline, not as crash screens
- Network errors are always wrapped as `Error("Network error")` — never undefined or uncaught
- Component-level error boundaries (where applicable) contain failures; one broken panel never crashes the page

## Recoverability
The UI returns to correct state automatically after failures or user re-engagement.
- Call `queryClient.invalidateQueries()` after every mutation — stale data is refetched immediately, no manual refresh needed
- Auth `onAuthStateChange` listener in `features/auth/store.ts` restores session after page refresh or tab focus — no re-login required
- `refetchInterval` on dashboard panels provides automatic recovery from transient API outages
- On 401 after token refresh, sign out and redirect to `/login` cleanly — no stuck state

## Faultlessness
Correctness is enforced by the type system and consistent patterns, not by discipline.
- No `any` types — all props, API responses, and state are explicitly typed
- All className composition uses `cn()` (clsx + tailwind-merge) — no conflicting Tailwind classes, no inline styles
- All props interfaces are defined and exported where reusable — never anonymous `{}` prop objects
- Static CONFIG maps over switch/if-else for display variants — adding a new case is one line, never a missed branch

## Maintainability
Features can be understood, changed, and extended without reading unrelated code.
- Feature-slice structure (`features/auth/`, `features/jobs/`, `features/companies/`, etc.) — each feature owns its components, hooks, and tests co-located
- Components have one concern — split when a component fetches AND renders AND manages local UI state
- `apiClient` is the single HTTP primitive — change auth behaviour in one file, it applies everywhere
- `lib/queryClient.ts` holds all TanStack Query defaults — tune retry/stale behaviour globally in one place

## Modularity
Each piece of the UI is independently understandable, replaceable, and testable.
- Presentational components (`StatusPill`, `KpiCard`, `DataTable`) carry no domain knowledge — they accept typed props and render, nothing else
- API functions in `api/*.ts` are pure — they call the backend and return typed data; no UI concerns, no side effects
- Custom hooks (e.g. `useAdapterConfigs`) encapsulate query logic — components consume the hook, not raw `useQuery`
- `App.tsx` owns routing only — no business logic, no data fetching

## Reusability
Shared code is written once and used everywhere; no copy-paste of logic or UI patterns.
- `apiClient.get/post/patch/delete` — shared HTTP primitives used by all API functions in `api/`
- Shared components in `components/ui/` and `components/shared/` — use before writing a new one
- `cn()` for every className composition — consistent merging, no duplication
- Custom hooks for recurring query patterns — if two components need the same query, extract a hook

## Analyzeability
The UI exposes enough structure to reason about state, diagnose bugs, and trace data flow.
- Query keys are structured string arrays (`['company', id]`, `['dashboard-market']`) — predictable, greppable, and easy to invalidate precisely
- Zustand store for auth is the single source of truth for session state — one place to inspect
- TypeScript strict mode catches type mismatches at compile time before they reach the browser
- TanStack Query DevTools in development — inspect cache, staleness, and query status without console logging

## Modifiability
The UI can be changed safely; adding a feature does not require touching unrelated code.
- Server state via TanStack Query only — adding a new API call means adding one query function + one hook; nothing else changes
- Adding a new feature = new folder under `features/` — no existing feature is touched
- Change an endpoint shape in `api/*.ts`; the consuming hook updates; the component is untouched
- Shared UI state (filters, selections that cross components) goes in Zustand — not prop-drilled, not in a parent component

## Testability
Every layer of the frontend can be verified in isolation.
- `apiClient` is mockable — tests replace `fetch` via `vi.fn()`, not the entire HTTP stack
- Use a test-specific `queryClient` — no global cache state leaks between tests
- Auth is mocked via Supabase stubs — components that depend on session are tested without real auth
- Hooks are testable via `renderHook` — extract query/mutation logic into hooks so components stay thin and testable
- No real network calls in unit tests — mock at the `fetch` boundary, not at the component boundary
