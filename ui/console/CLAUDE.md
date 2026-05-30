# JobRadar Console — Frontend Architecture

## Stack

React 19 · TypeScript (strict) · Vite · TanStack Query v5 · TanStack Table v8 · Zustand · Tailwind · Radix UI · Recharts · Supabase Auth · Vitest + Testing Library.

## Project layout

`src/config/` (env, routes, constants) · `src/lib/` (api-client, queryClient, supabase, utils, commands) · `src/types/` (all shared types in `index.ts`) · `src/features/<domain>/` (api + components + hooks + `queryKeys.ts` + `__tests__/` + barrel `index.ts`) · `src/components/ui/` (generic, zero domain knowledge) · `src/components/shared/` (lightly domain-aware) · `src/pages/` (route-level, compose features only).

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

90% statement coverage per file. Run: `cd ui/console && npx vitest run --coverage`.

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
