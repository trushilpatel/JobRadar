# JobRadar Console — Frontend Architecture

## Stack

React 19 · TypeScript (strict) · Vite · TanStack Query v5 · TanStack Table v8 · Zustand · Tailwind · Radix UI · Recharts · Clerk Auth · Vitest + Testing Library.

## Project layout

`src/config/` (env, routes, constants) · `src/lib/` (api-client, queryClient, resilient-parse, utils, commands) · `src/types/` (all shared types in `index.ts`) · `src/features/<domain>/` (api + components + hooks + `queryKeys.ts` + `__tests__/` + barrel `index.ts`) · `src/components/ui/` (generic, zero domain knowledge) · `src/components/shared/` (lightly domain-aware) · `src/pages/` (route-level, compose features only).

---

## Canonical patterns

### 1. API functions (`src/features/<domain>/api/<domain>.api.ts`)

- Return a **fully normalized, non-null** type — never `T | null`
- Normalize null/undefined inside the function body with `?? fallback`
- Use `apiClient.get<T>` with an explicit generic — no UI logic, toasts, or cache manipulation

```ts
// GOOD — normalization stays in the API function
export async function fetchTechStack(limit = 15): Promise<TechCount[]> {
  const result = await apiClient.get<TechCount[] | null>(
    `/admin/insights/tech-stack?limit=${limit}`
  )
  return result ?? []
}
```

**Generated backend types (`src/lib/api/schema.gen.ts`):** auto-generated from `backend/apps/api/openapi.yaml` via `npm run gen:api-types` (`openapi-typescript`) — never hand-edit it; CI regenerates and diffs it against the committed file (drift fails the build, mirroring the backend's own stale-spec guard). This does **not** replace the zod-schema-per-endpoint pattern for _response_ parsing — zod stays the runtime source of truth for anything coming back over the wire, since the generated types are compile-time only and openapi-typescript doesn't validate at runtime. Use it for **request bodies**: build the body as a normal object, then `satisfies components['schemas']['SomeInputBody']` on it so a renamed/removed backend field fails the frontend build instead of silently drifting (see `setUserRoles`/`setUserStatus` in `features/auth/api/adminUsers.api.ts`, `triggerRetry` in `lib/api/pipeline.ts` for the pattern). Not yet rolled out to every `.api.ts` file — apply it opportunistically when touching a file with an untyped request body, don't do a blanket rewrite pass.

### 2. Query key factories (`src/features/<domain>/queryKeys.ts`)

Every domain owns its `queryKeys.ts` with typed `as const` tuples. Components and hooks **never** inline strings — always import from the factory.

```ts
export const insightQueryKeys = {
  techStack: (limit: number) => ['insights', 'tech-stack', limit] as const,
  funnel: () => ['insights', 'funnel'] as const,
}
```

### 3. Data hooks (`src/features/<domain>/hooks/use<Entity>.ts`)

Every `useQuery` / `useMutation` / `useInfiniteQuery` call lives in a named custom hook. Components never call TanStack Query primitives directly. Action hooks bundle related mutations (e.g. `useJobActions` returns `{ setStatus, dismiss }`).

### 4. Component pattern — Container → Content

Split every data-fetching component into two layers.

- **Container** calls the hook and renders loading / error / content states.
- **Content** is a pure component receiving typed props — no data fetching.

```tsx
export function TechStackPanel() {
  const { data, isLoading, isError } = useTechStack(15)
  return (
    <InsightCard title="Tech Stack Trends">
      {isLoading ? (
        <InsightSkeleton />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <TechContent data={data} />
      )}
    </InsightCard>
  )
}
```

Rules:

- `data` from `useQuery` is `T | undefined` and is `undefined` only during loading/error. After the `isLoading / isError / !data` guard, TypeScript narrows to `T`.
- Never use `const { data = [] }` — it hides the undefined state from TypeScript.
- Never pass `data ?? []` to a content component when the API function already normalises — trust the normalisation.
- Empty data is the content component's call (render `InsightEmpty`).

### 5. State management

| State type                  | Tool           |
| --------------------------- | -------------- |
| Server state (API data)     | TanStack Query |
| App-wide session / identity | Zustand (auth) |
| Cross-component UI state    | Zustand        |
| Component-local UI state    | `useState`     |

Never use `useState` + `useEffect` + `fetch` for server data. Always `useQuery`.

### 6. Types (`src/types/index.ts`)

Single source of truth. No `any` anywhere; no anonymous `{}` prop types — define and export named interfaces. Union types for status/stage/reason values, never raw strings. All API response shapes typed even when fields are optional.

### 7. Error handling

`apiClient` parses errors into `ApiValidationError`; components render the error state via the `isError` flag. `ApiValidationError.fieldErrors()` returns `Record<string, string>` for form use. Never let an API error crash the UI — always handle `isError`.

### 8. Loading states

Every fetching panel renders a skeleton while `isLoading` is true. Never leave a panel blank. Use `<InsightSkeleton>`, `animate-pulse` divs, or equivalent.

### 9. Mutations + cache invalidation

```ts
const setStatus = useMutation({
  mutationFn: ({ id, status }: { id: string; status: JobStatus }) => setJobStatus(id, status),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: jobQueryKeys.list() })
    queryClient.invalidateQueries({ queryKey: jobQueryKeys.counts() })
    toast.success('Updated')
  },
  onError: () => toast.error('Failed to update'),
})
```

- Always invalidate affected queries on success — keep targeted, use the factory (not `invalidateQueries()` with no args)
- Toast feedback is required for all mutations

### 10. Styling

- **Only** `cn()` (clsx + tailwind-merge) for `className` composition — never string concat
- Tailwind classes only — no inline styles
- Radix UI for accessible primitives (dialogs, selects, tooltips, switches)
- **Theme-adaptive colors in JS must use CSS custom properties** — when a color value is used in a JS/TS context (inline `style` prop, Recharts `stroke`/`fill`, sparkline background arrays), it cannot reference Tailwind's `dark:` prefix. Define the color pair in `index.css` (`:root` = light, `.dark` = dark) and reference it as `'var(--name)'`. The `dark:` prefix is acceptable only inside Tailwind `className` strings on DOM elements.
- **Recharts accepts CSS variable strings** — `stroke`, `fill`, and tick `fill` props accept `'var(--text-muted)'`, `'var(--border)'`, etc. Use them instead of hardcoded hex to keep charts readable in both themes.
- **`InsightEmpty` vs `InsightError` must be visually distinct** — empty state uses `text-placeholder`; error state uses an `AlertCircle` icon + `bg-red-500/5 border-red-500/10` background. Never style them identically.
- **Brand color tokens (`violet`, `cyan`, `magenta`, `lime`, `ok`, `warn`, `danger`, `remote`, `salary`, etc.) are flat single values in `tailwind.config.js`** (`violet: 'var(--violet)'`), not numbered shade scales — they override Tailwind's built-in scale for that key. Classes like `bg-violet-600`, `text-violet-300`, `border-cyan-400/50` silently generate **no CSS at all** (Tailwind emits nothing for an unrecognized shade suffix, and the browser just ignores the unknown class — no error, no warning). Use the bare token (`bg-violet`, `text-cyan`) for solid fills — it already adapts between light/dark via the underlying CSS var — and `bg-[color-mix(in_srgb,var(--violet)_14%,transparent)]` (see `jobStatusConfig.ts` for the established pattern) for a tinted/translucent background instead of an opacity-modified numbered shade. This bug shipped with the original token re-skin and silently broke checked-checkbox fills, active-tab tints, and button hover states across multiple already-shipped components before being caught and fixed in the jobs-page redesign (2026-07-03) — audit any `-\d\d\d` suffix on a brand color token you encounter.

---

## Anti-patterns

| Anti-pattern                                         | Why                                                                                           | Fix                                            |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| `const { data = [] } = useQuery(...)`                | Hides the `undefined` state from TypeScript; impossible to distinguish "loading" from "empty" | Handle undefined via `isLoading / !data` guard |
| `select: (d: T \| null) => d ?? []` in components    | Null normalization belongs in the API function                                                | Move `?? fallback` into `fetchXxx`             |
| Inline query key strings (`['insights-tech-stack']`) | Typos undetected; invalidation becomes fragile grep-work                                      | Import from `queryKeys.ts`                     |
| `useQuery` called directly in a component            | Couples component to query config; impossible to mock the hook in isolation                   | Extract to a named hook                        |
| `fetch` called directly                              | Bypasses auth, error handling, envelope unwrapping, timeout, retry                            | Always use `apiClient`                         |
| `any` type                                           | Defeats TypeScript                                                                            | Type everything; use `unknown` at boundaries   |
| Data-fetching in pages (`src/pages/`)                | Pages compose features, they don't own data                                                   | Move queries into feature hooks                |
| `useEffect` + `useState` for API calls               | Misses loading/error states, auth, caching, dedup                                             | Use `useQuery`                                 |

---

## Testing

### Layers + mock points

| Layer         | Tool                                              | Mock                                                                        |
| ------------- | ------------------------------------------------- | --------------------------------------------------------------------------- |
| API functions | Vitest                                            | `apiClient.get/post/patch/delete` via `vi.fn()`                             |
| Custom hooks  | `renderHook` + `QueryClientProvider`              | The underlying `fetchXxx` function                                          |
| Components    | `render` + `QueryClientProvider`                  | The `fetchXxx` function (`vi.mock('@/features/<domain>/api/<domain>.api')`) |
| Pages         | `render` + `MemoryRouter` + `QueryClientProvider` | All `fetchXxx` functions used by child panels                               |

### Conventions

- Each test file builds its own `QueryClient` (with `retry: 0, gcTime: 0`) to prevent cache bleed
- Mock at the API module level (`vi.mock('@/features/insights/api/insights.api')`) — never at fetch level
- For API functions, test the normalisation explicitly: a server `null` must resolve to `[]`
- Every panel covers the four states it must handle: loading, populated, empty, error
- Every hook covers: successful data transformation, error propagation, mutation success + cache invalidation, mutation error handling

### Coverage target

90% statement coverage per file. Run: `cd frontend && npx vitest run --coverage`.

### End-to-end (Playwright)

`frontend/e2e/` is a separate, full-flow browser suite — a distinct tier from the
unit tests above, not a replacement. Covers every page except the mobile-only
`/jobs/:id` route, for both a regular user and an admin. See `frontend/e2e/CLAUDE.md`
for the full runbook, conventions (Page Object Model, auth via cached `storageState`,
data-safety rules), and known gaps.

```bash
make dev                    # start the local stack first (frontend/e2e/ doesn't manage it)
cd frontend && npm run test:e2e         # full suite, headless
npm run test:e2e:ui                     # Playwright's interactive UI mode
npm run test:e2e:report                 # open the last HTML report
```

Not wired into CI yet — run it manually when touching a flow it covers.

### ResizeObserver mock (required for Recharts)

```ts
// src/test/setup.ts
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver
```

---

## File naming

| Type            | Convention                             | Example                              |
| --------------- | -------------------------------------- | ------------------------------------ |
| Components      | PascalCase `.tsx`                      | `TechStackPanel.tsx`                 |
| Hooks           | camelCase with `use` prefix            | `useTechStack.ts`                    |
| API functions   | camelCase with `.api.ts` suffix        | `insights.api.ts`                    |
| Query key files | `queryKeys.ts`                         | `src/features/insights/queryKeys.ts` |
| Test files      | `<subject>.test.ts(x)` in `__tests__/` | `TechStackPanel.test.tsx`            |
| Constants       | camelCase                              | `jobStatus.ts`                       |

---

## UI Component Rules (enforced for all contributors)

### The boundary: `components/ui/` is display-only

Every file in `src/components/ui/` must satisfy one of two conditions:

**a. shadcn/ui origin** — added via `npx shadcn@latest add <name>` and styled to match this project's design tokens. Do not restructure shadcn components; only restyle with Tailwind classes and `cn()`. If the shadcn component needs new behaviour, wrap it in a feature-level component — never edit the shadcn source.

**b. Pure custom** — wraps a Radix primitive or is hand-built. Must satisfy ALL of the following:

- No `useEffect` that calls a prop callback (no `onFirst()`, no `onReset()`, no parent mutations)
- No internal state that drives parent navigation or data decisions
- No business logic ("if itemsOnPage === 0 call onFirst()" is business logic)
- Contract: **data in → render out → event callbacks out**. Nothing else.

Any component that does not satisfy (a) or (b) is a design error. The fix is always the same: move the logic to the hook or service that owns the domain.

### New component decision tree (mandatory — no exceptions)

1. **Does shadcn/ui have it?** → `npx shadcn@latest add <name>`. Done. Do not write a custom version.
2. **Does Radix UI have a headless primitive?** → write a thin styled wrapper using Tailwind + `cn()`. Done.
3. **Neither?** → write a pure custom component. The PR description MUST include one sentence explaining why shadcn and Radix do not cover this use case.

When in doubt: check https://ui.shadcn.com/docs/components (no account needed).

### Forbidden patterns

These are hard rules. Violations block merge.

| Pattern                                                          | Forbidden because                                                                       | Correct alternative                                                                                  |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `createPortal` directly in a component                           | Accessibility, z-index, focus management bugs                                           | Radix `Portal` or shadcn `Popover`/`Dialog` which handle this correctly                              |
| `useInfiniteQuery` for page-by-page cursor navigation            | Designed for infinite scroll; creates async race between cursor and loading state       | `useQuery` + explicit `cursorHistory: string[]` in hook state (see `features/jobs/hooks/useJobs.ts`) |
| Multi-select / combobox / filterable dropdown built from scratch | Broken keyboard nav, missing ARIA roles, brittle portal logic                           | shadcn `Popover + Command + Checkbox` — the canonical pattern for this project                       |
| `useEffect` with a side effect inside `components/ui/`           | Makes the component non-pure; test coverage becomes incomplete; creates hidden coupling | Move the logic to the feature hook that owns the domain (`useJobs`, `useCompanies`, etc.)            |
| `data = useQuery(...)?.data ?? []` to default in the component   | Hides `undefined` from TypeScript; impossible to distinguish loading from empty         | Handle in the API function normaliser; guard with `isLoading / isError / !data` in the component     |

### Cursor pagination — canonical pattern

All paginated list views use `useQuery` with an explicit cursor history. Do not use `useInfiniteQuery` for navigation.

```ts
// hooks/useXxx.ts  ← hooks own ALL pagination state and decisions

// cursorHistory[pageIndex] is the cursor for the currently displayed page.
// Stored in sessionStorage (keyed by filter fingerprint) so same-session
// back/forward works without refetching. Resets to [''] on any filter change.
const [cursorHistory, setCursorHistory] = useState<string[]>(() =>
  restoreHistory(filterFingerprint)
)
const currentCursor = cursorHistory[pageIndex] ?? ''

const query = useQuery({
  queryKey: [...filters, currentCursor, limit],
  queryFn: ({ signal }) => fetchXxx(filters, currentCursor, limit, signal),
  staleTime: 30_000,
})

function goToNextPage() {
  const nextCursor = query.data?.next_cursor
  if (!nextCursor) return // last page — no-op
  const extended = cursorHistory.slice(0, pageIndex + 1).concat(nextCursor)
  setCursorHistory(extended)
  persistHistory(filterFingerprint, extended)
  void setPageIndex(pageIndex + 1)
}

function goToPrevPage() {
  if (pageIndex > 0) void setPageIndex(pageIndex - 1) // cursor already in history
}

function resetCursor() {
  // call on any filter/sort/limit change
  setCursorHistory([''])
  clearHistory(filterFingerprint)
  void setPageIndex(0)
}
```

The `Pagination` component is a **pure display component**: it receives `hasNext`, `hasPrev`, `page`, `itemsOnPage`, `itemsBeforePage` and fires `onNext`/`onPrev`/`onFirst` callbacks. It makes no decisions. The hook decides when to call `goToFirstPage`.

### shadcn component registry (currently installed)

Run `npx shadcn@latest add <name>` to install. Do not copy-paste from shadcn docs manually.

Already installed: `alert-dialog` · `badge` · `button` · `card` · `checkbox` · `command` · `dialog` · `drawer` · `input` · `label` · `popover` · `scroll-area` · `separator` · `sheet` · `skeleton` · `switch` · `tabs` · `toggle` · `toggle-group` · `tooltip`

Custom (no shadcn equivalent, stays custom): `pagination` · `data-table` · `resize-handle` · `dismiss-stripe` · `scan-history-bar` · `kbd`
