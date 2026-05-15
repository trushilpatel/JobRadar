# JobRadar Console — Frontend Architecture

## Tech stack

React 19 · TypeScript (strict) · Vite · TanStack Query v5 · TanStack Table v8 · Zustand · Tailwind CSS · Radix UI · Recharts · Supabase Auth · Vitest + Testing Library

---

## Project structure

```
src/
├── api/                  # API call functions — one file per domain
├── types/                # Centralized TypeScript interfaces (types/index.ts)
├── constants/            # Shared constants (jobStatus, routes)
├── lib/                  # Infrastructure (api-client, queryClient, supabase, utils)
├── features/             # Domain-organized feature modules
│   └── <domain>/
│       ├── components/   # Domain-specific components
│       ├── hooks/        # Custom hooks (data-fetching + actions)
│       ├── queryKeys.ts  # Query key factory for this domain
│       ├── __tests__/    # Tests co-located with the feature
│       └── index.ts      # Barrel export
├── components/
│   ├── ui/               # Generic, reusable, zero domain knowledge
│   └── shared/           # Lightly domain-aware shared components
└── pages/                # Route-level components — compose features only
```

---

## Canonical patterns

### 1. API functions (`src/api/<domain>.ts`)

Every fetch function:

- Returns a **fully normalized, non-null type** — never `T | null`
- Normalizes null/undefined from the server with `?? fallback` in the function body
- Uses `apiClient.get<T>` with an explicit generic
- Does not contain UI logic, toasts, or cache manipulation

```ts
// GOOD
export async function fetchTechStack(limit = 15): Promise<TechCount[]> {
  const result = await apiClient.get<TechCount[] | null>(
    `/admin/insights/tech-stack?limit=${limit}`
  )
  return result ?? []
}

// BAD — leaks null to callers, forces every component to guard against null
export async function fetchTechStack(limit = 15): Promise<TechCount[] | null> {
  return apiClient.get<TechCount[] | null>(`/admin/insights/tech-stack?limit=${limit}`)
}
```

### 2. Query key factories (`src/features/<domain>/queryKeys.ts`)

Every domain has its own `queryKeys.ts` file. Keys are typed `as const` tuples.

```ts
// src/features/insights/queryKeys.ts
export const insightQueryKeys = {
  techStack: (limit: number) => ['insights', 'tech-stack', limit] as const,
  companyActivity: () => ['insights', 'company-activity'] as const,
  funnel: () => ['insights', 'funnel'] as const,
  location: () => ['insights', 'location'] as const,
  sponsorshipTrend: (days: number) => ['insights', 'sponsorship-trend', days] as const,
  velocity: (limit: number) => ['insights', 'velocity', limit] as const,
  roleCategories: () => ['insights', 'role-categories'] as const,
}
```

**Never use inline strings** for query keys in components or hooks. Always import from the domain's `queryKeys.ts`.

### 3. Data-fetching hooks (`src/features/<domain>/hooks/use<Entity>.ts`)

Every `useQuery` / `useMutation` / `useInfiniteQuery` call lives in a named custom hook. Components never call TanStack Query primitives directly.

```ts
// src/features/insights/hooks/useTechStack.ts
export function useTechStack(limit = 15) {
  return useQuery({
    queryKey: insightQueryKeys.techStack(limit),
    queryFn: () => fetchTechStack(limit),
    staleTime: 30_000,
  })
}
```

Action hooks bundle related mutations:

```ts
// src/features/jobs/hooks/useJobActions.ts
export function useJobActions() {
  const setStatus = useMutation({ ... })
  const dismiss   = useMutation({ ... })
  return { setStatus, dismiss }
}
```

### 4. Component pattern (Container → Content)

Split every data-fetching component into two layers:

**Container** — calls the hook, renders loading / error / content states:

```tsx
export function TechStackPanel() {
  const { data, isLoading, isError } = useTechStack(15)
  return (
    <InsightCard title="Tech Stack Trends" subtitle="Top 15 technologies">
      {isLoading ? (
        <InsightSkeleton height="h-48" />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <TechContent data={data} />
      )}
    </InsightCard>
  )
}
```

**Content** — pure component, receives typed props, no data fetching:

```tsx
function TechContent({ data }: { data: TechCount[] }) {
  if (data.length === 0) return <InsightEmpty />
  return <BarChart ... />
}
```

**Rules:**

- `data` from `useQuery` is `T | undefined`. It is `undefined` only while loading or in error state. After the `isLoading / isError / !data` guard, TypeScript knows it is `T`.
- Never use `const { data = [] }` destructuring defaults — this masks the undefined state from TypeScript and creates a false sense of safety.
- Never pass `data ?? []` to a content component when the API function already guarantees a non-null return — trust the normalization.
- If data truly can be empty (e.g., an empty list), let the content component decide how to render that (`InsightEmpty`).

### 5. State management

| State type                  | Tool           |
| --------------------------- | -------------- |
| Server state (API data)     | TanStack Query |
| App-wide session / identity | Zustand (auth) |
| Cross-component UI state    | Zustand        |
| Component-local UI state    | `useState`     |

Never use `useState` + `useEffect` + `fetch` for server data. Always use `useQuery`.

### 6. Types (`src/types/index.ts`)

All shared domain interfaces live in `src/types/index.ts`. This is the single source of truth.

- No `any` types anywhere
- No anonymous `{}` prop types — define and export named interfaces
- Union types for status/stage/reason values — no raw strings
- All API response shapes are typed even if the API may omit optional fields

### 7. Error handling

- API errors → `apiClient` parses them into `ApiValidationError`; components display the error state
- Query errors → `isError` flag from `useQuery`; all panels render an `<InsightError />` or equivalent
- Field-level validation → `ApiValidationError.fieldErrors()` returns a `Record<string, string>` for form use
- Never let an API error crash the UI — always handle `isError`

### 8. Loading states

Every panel that fetches data **must** render a loading skeleton while `isLoading` is true. Never leave a panel blank. Use `<InsightSkeleton>`, `animate-pulse` divs, or equivalent skeleton placeholders.

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

- Always invalidate affected queries on success
- Keep invalidation targeted — use the query key factory, not `queryClient.invalidateQueries()`
- Toast feedback is required for all mutations

### 10. Styling

- **Only** `cn()` (clsx + tailwind-merge) for all `className` composition — never string concatenation
- Tailwind classes only — no inline styles
- Radix UI for accessible primitives (dialogs, selects, tooltips, switches)

---

## Anti-patterns

| Anti-pattern                                         | Why                                                                                                    | Fix                                             |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------ | ----------------------------------------------- |
| `const { data = [] } = useQuery(...)`                | Hides the `undefined` state from TypeScript; makes it impossible to distinguish "loading" from "empty" | Handle undefined via `isLoading / !data` guard  |
| `select: (d: T \| null) => d ?? []` in components    | Null normalization belongs in the API function                                                         | Move `?? fallback` into the `fetchXxx` function |
| Inline query key strings (`['insights-tech-stack']`) | Typos go undetected; invalidation becomes fragile grep-work                                            | Import from `queryKeys.ts`                      |
| `useQuery` called directly in a component            | Couples component to query config; impossible to mock the hook in isolation                            | Extract to a named hook                         |
| `fetch` called directly                              | Bypasses auth, error handling, envelope unwrapping, timeout, retry                                     | Always use `apiClient`                          |
| `any` type                                           | Defeats TypeScript                                                                                     | Type everything; use `unknown` at boundaries    |
| Data-fetching in pages (`src/pages/`)                | Pages should compose features, not own data                                                            | Move queries into feature hooks                 |
| `useEffect` + `useState` for API calls               | Misses loading/error states, auth, caching, dedup                                                      | Use `useQuery`                                  |

---

## Testing

### Layers

| Layer         | Tool                                              | What to mock                                                   |
| ------------- | ------------------------------------------------- | -------------------------------------------------------------- |
| API functions | Vitest                                            | `apiClient.get/post/patch/delete` via `vi.fn()`                |
| Custom hooks  | `renderHook` + `QueryClientProvider`              | The underlying `fetchXxx` function                             |
| Components    | `render` + `QueryClientProvider`                  | The `fetchXxx` function (mock via `vi.mock('@/api/<domain>')`) |
| Pages         | `render` + `MemoryRouter` + `QueryClientProvider` | All `fetchXxx` functions used by child panels                  |

### Conventions

```ts
// 1. Each test file gets its own QueryClient to prevent cache bleed
function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: 0 } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

// 2. Mock at the API module level — not at fetch level
vi.mock('@/api/insights')

// 3. Test the four states every panel must handle
it('shows skeleton while loading', () => { ... })
it('renders content when data is present', async () => { ... })
it('shows empty state when data is []', async () => { ... })
it('shows error state when query rejects', async () => { ... })

// 4. For API functions: test that normalization works
it('normalizes null server response to []', async () => {
  vi.mocked(apiClient.get).mockResolvedValue(null)
  await expect(fetchTechStack(15)).resolves.toEqual([])
})
```

### Coverage target

90% statement coverage per file. Run: `cd ui/console && npx vitest run --coverage`

Every new component ships with tests covering at minimum:

- Loading state
- Populated state (renders key data)
- Empty state
- Error state

Every new hook ships with tests covering:

- Successful data transformation
- Error propagation
- Mutation success + cache invalidation
- Mutation error handling

### ResizeObserver mock (required for Recharts)

```ts
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver
```

Place this in `src/test/setup.ts` so all tests have it automatically.

---

## File naming

| Type            | Convention                             | Example                              |
| --------------- | -------------------------------------- | ------------------------------------ |
| Components      | PascalCase `.tsx`                      | `TechStackPanel.tsx`                 |
| Hooks           | camelCase with `use` prefix            | `useTechStack.ts`                    |
| API functions   | camelCase                              | `insights.ts`                        |
| Query key files | `queryKeys.ts`                         | `src/features/insights/queryKeys.ts` |
| Test files      | `<subject>.test.ts(x)` in `__tests__/` | `TechStackPanel.test.tsx`            |
| Constants       | camelCase                              | `jobStatus.ts`                       |
