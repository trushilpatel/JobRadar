# Frontend Restructure — Component-Based Architecture with React Query

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the monolithic `JobsPage.tsx` into a scalable feature-based structure with a services layer and React Query for data fetching/caching.

**Architecture:** `api/` stays as the raw HTTP layer. `services/` holds pure business logic (formatSalary, avatarColor). `features/jobs/` is self-contained with components and React Query hooks. `pages/JobsPage.tsx` becomes a thin shell composing feature components. `lib/queryClient.ts` configures the global React Query client.

**Tech Stack:** React 19, @tanstack/react-query v5, TypeScript 5, Tailwind CSS v3, Vite 5, sonner (toasts)

> **Note on testing:** No test infrastructure (vitest/jest) exists in this project. TDD steps are replaced with `tsc --noEmit` type-checks as the verification gate.

---

## File Map

**Create:**
- `frontend/src/lib/queryClient.ts`
- `frontend/src/services/jobs.service.ts`
- `frontend/src/components/shared/CompanyAvatar.tsx`
- `frontend/src/features/jobs/hooks/useJobs.ts`
- `frontend/src/features/jobs/hooks/useJobActions.ts`
- `frontend/src/features/jobs/components/DismissModal.tsx`
- `frontend/src/features/jobs/components/JobStatusTabs.tsx`
- `frontend/src/features/jobs/components/JobFilters.tsx`
- `frontend/src/features/jobs/components/jobs-columns.tsx`
- `frontend/src/features/jobs/components/JobsTable.tsx`
- `frontend/src/features/jobs/components/JobDetailPanel.tsx`
- `frontend/src/features/jobs/index.ts`

**Modify:**
- `frontend/src/App.tsx` — wrap with `QueryClientProvider`
- `frontend/src/pages/JobsPage.tsx` — complete rewrite as thin shell

**Delete:**
- `frontend/src/pages/jobs-columns.tsx` — moves to `features/jobs/components/`

---

## Task 1: Install React Query + create QueryClient

**Files:**
- Modify: `frontend/package.json` (via npm install)
- Create: `frontend/src/lib/queryClient.ts`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Install @tanstack/react-query**

```bash
cd frontend && npm install @tanstack/react-query
```

Expected: package installs cleanly, `@tanstack/react-query` appears in `package.json` dependencies.

- [ ] **Step 2: Create `src/lib/queryClient.ts`**

```ts
import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 30,      // 30s — jobs data is fresh for 30 seconds
      gcTime: 1000 * 60 * 5,    // 5min — keep unused data in cache for 5 minutes
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
```

- [ ] **Step 3: Wrap App with QueryClientProvider**

Replace the contents of `frontend/src/App.tsx`:

```tsx
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/queryClient'
import { Sidebar } from '@/components/Sidebar'
import { CompaniesPage } from '@/pages/CompaniesPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { ApplicationsPage } from '@/pages/ApplicationsPage'
import { InsightsPage } from '@/pages/InsightsPage'

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <HashRouter>
        <Toaster position="bottom-center" theme="dark" richColors />
        <div className="flex h-screen w-full overflow-hidden">
          <Sidebar />
          <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/companies" element={<CompaniesPage />} />
              <Route path="/jobs" element={<JobsPage />} />
              <Route path="/applications" element={<ApplicationsPage />} />
              <Route path="/insights" element={<InsightsPage />} />
            </Routes>
          </main>
        </div>
      </HashRouter>
    </QueryClientProvider>
  )
}
```

- [ ] **Step 4: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
cd frontend && git add src/lib/queryClient.ts src/App.tsx package.json package-lock.json
git commit -m "feat(frontend): install React Query and wire up QueryClientProvider"
```

---

## Task 2: Create `services/jobs.service.ts`

Extract pure utility functions from `JobsPage.tsx` and `jobs-columns.tsx` into a single source of truth. These functions have no React dependencies.

**Files:**
- Create: `frontend/src/services/jobs.service.ts`

- [ ] **Step 1: Create `src/services/jobs.service.ts`**

```ts
import type { SalaryRange } from '@/types'

// ── Avatar color ──────────────────────────────────────────────────────────────

const AVATAR_COLORS = [
  'bg-violet-600', 'bg-blue-600', 'bg-emerald-600',
  'bg-amber-600',  'bg-rose-600', 'bg-cyan-600',
  'bg-indigo-600', 'bg-teal-600',
]

export function avatarColor(name: string): string {
  if (!name) return AVATAR_COLORS[0]
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

// ── Salary formatting ─────────────────────────────────────────────────────────

export function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  const currency = salary.currency ?? '$'
  const fmt = (n: number) =>
    n >= 1000 ? `${currency}${Math.round(n / 1000)}k` : `${currency}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/services/jobs.service.ts
git commit -m "feat(frontend): add jobs.service.ts with formatSalary and avatarColor"
```

---

## Task 3: Create `components/shared/CompanyAvatar.tsx`

Deduplicates `CompanyAvatar` that exists identically in both `JobsPage.tsx` and `jobs-columns.tsx`.

**Files:**
- Create: `frontend/src/components/shared/CompanyAvatar.tsx`

- [ ] **Step 1: Create `src/components/shared/CompanyAvatar.tsx`**

```tsx
import { cn } from '@/lib/utils'
import { avatarColor } from '@/services/jobs.service'

interface CompanyAvatarProps {
  name: string
  size?: 'sm' | 'md'
}

export function CompanyAvatar({ name, size = 'sm' }: CompanyAvatarProps) {
  return (
    <div
      className={cn(
        'rounded-md flex items-center justify-center font-semibold text-white shrink-0',
        avatarColor(name),
        size === 'sm' ? 'w-7 h-7 text-xs' : 'w-9 h-9 text-sm',
      )}
    >
      {name ? name[0].toUpperCase() : '?'}
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/components/shared/CompanyAvatar.tsx
git commit -m "feat(frontend): add shared CompanyAvatar component"
```

---

## Task 4: Create `features/jobs/hooks/useJobs.ts`

React Query `useQuery` wrapper that also owns all filter state and pagination. Replaces the `useState + useEffect` data fetching pattern in `JobsPage`.

**Files:**
- Create: `frontend/src/features/jobs/hooks/useJobs.ts`

- [ ] **Step 1: Create the directory structure**

```bash
mkdir -p frontend/src/features/jobs/hooks
mkdir -p frontend/src/features/jobs/components
```

- [ ] **Step 2: Create `src/features/jobs/hooks/useJobs.ts`**

```ts
import { useState, useMemo, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchJobs, type JobFilters } from '@/api/jobs'

export type TabKey = 'new' | 'saved' | 'applied'
export type DateFilter = 'all' | '7d' | '30d'

export function useJobs() {
  // Tab
  const [tab, setTab] = useState<TabKey>('new')

  // Filter state — mirrors existing defaults in JobsPage
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>([])
  const [remoteFilter, setRemoteFilter]               = useState(false)
  const [employTypes, setEmployTypes]                 = useState<string[]>(['full_time'])
  const [sponsorshipFilter, setSponsorshipFilter]     = useState(true)
  const [noSponsorshipFilter, setNoSponsorshipFilter] = useState(false)
  const [noRequirementsFilter, setNoRequirementsFilter] = useState(true)
  const [citizenshipReqFilter, setCitizenshipReqFilter] = useState(false)
  const [expFilter, setExpFilter]                     = useState<string[]>(['0-2', '3-5'])
  const [dateFilter, setDateFilter]                   = useState<DateFilter>('all')

  // Pagination
  const [page, setPage]   = useState(1)
  const [limit, setLimit] = useState(20)

  // Stable filter object — only changes when a filter value changes
  const filters = useMemo<JobFilters>(() => ({
    status: tab,
    companies: selectedCompanies.length > 0 ? selectedCompanies : undefined,
    remote: remoteFilter || undefined,
    employTypes: employTypes.length > 0 ? employTypes : undefined,
    sponsorship: sponsorshipFilter ? true : noSponsorshipFilter ? false : undefined,
    noRequirements: noRequirementsFilter || undefined,
    requiresCitizen: citizenshipReqFilter ? true : undefined,
    experienceRanges: expFilter.length > 0 ? expFilter : undefined,
    daysAdded: dateFilter === '7d' ? 7 : dateFilter === '30d' ? 30 : undefined,
  }), [
    tab, selectedCompanies, remoteFilter, employTypes,
    sponsorshipFilter, noSponsorshipFilter, noRequirementsFilter,
    citizenshipReqFilter, expFilter, dateFilter,
  ])

  // Reset to page 1 when filters change
  useEffect(() => { setPage(1) }, [filters])

  const query = useQuery({
    queryKey: ['jobs', filters, page, limit],
    queryFn: () => fetchJobs(filters, page, limit),
  })

  const jobs  = query.data?.data  ?? []
  const total = query.data?.total ?? 0

  // Companies for the multi-select dropdown — derived from current page results
  const companies = useMemo(() => {
    const names = Array.from(
      new Set(jobs.map(j => j.company?.name ?? j.company_id).filter(Boolean))
    )
    return (names as string[]).sort()
  }, [jobs])

  function resetFilters() {
    setSelectedCompanies([])
    setRemoteFilter(false)
    setSponsorshipFilter(true)
    setNoSponsorshipFilter(false)
    setNoRequirementsFilter(true)
    setCitizenshipReqFilter(false)
    setExpFilter(['0-2', '3-5'])
    setDateFilter('all')
    setPage(1)
  }

  return {
    // Query state
    jobs,
    total,
    companies,
    isLoading: query.isLoading,
    refetch: query.refetch,

    // Tab
    tab, setTab,

    // Filters
    selectedCompanies, setSelectedCompanies,
    remoteFilter,      setRemoteFilter,
    employTypes,       setEmployTypes,
    sponsorshipFilter,    setSponsorshipFilter,
    noSponsorshipFilter,  setNoSponsorshipFilter,
    noRequirementsFilter, setNoRequirementsFilter,
    citizenshipReqFilter, setCitizenshipReqFilter,
    expFilter,  setExpFilter,
    dateFilter, setDateFilter,
    resetFilters,

    // Pagination
    page, setPage,
    limit, setLimit,
  }
}
```

- [ ] **Step 3: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
cd frontend && git add src/features/
git commit -m "feat(frontend): add useJobs hook with React Query and filter state"
```

---

## Task 5: Create `features/jobs/hooks/useJobActions.ts`

React Query `useMutation` wrappers for save, apply, dismiss, and bulk dismiss. Invalidates the `['jobs']` query key on success so React Query refetches automatically.

**Files:**
- Create: `frontend/src/features/jobs/hooks/useJobActions.ts`

- [ ] **Step 1: Create `src/features/jobs/hooks/useJobActions.ts`**

```ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { setJobSaved, setJobStatus, dismissJob } from '@/api/jobs'
import { toast } from 'sonner'

export function useJobActions() {
  const queryClient = useQueryClient()

  function invalidateJobs() {
    queryClient.invalidateQueries({ queryKey: ['jobs'] })
  }

  const save = useMutation({
    mutationFn: ({ id, saved }: { id: string; saved: boolean }) =>
      setJobSaved(id, saved),
    onSuccess: (_, { saved }) => {
      toast.success(saved ? 'Job saved' : 'Removed from saved')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
  })

  const apply = useMutation({
    mutationFn: (id: string) => setJobStatus(id, 'applied'),
    onSuccess: () => {
      toast.success('Marked as applied')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to update job'),
  })

  const dismiss = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      dismissJob(id, reason),
    onSuccess: () => {
      toast.success('Job dismissed')
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss job'),
  })

  const bulkDismiss = useMutation({
    mutationFn: (ids: string[]) =>
      Promise.all(ids.map(id => dismissJob(id, 'user_not_relevant'))),
    onSuccess: (_, ids) => {
      toast.success(`${ids.length} job${ids.length !== 1 ? 's' : ''} dismissed`)
      invalidateJobs()
    },
    onError: () => toast.error('Failed to dismiss some jobs'),
  })

  return { save, apply, dismiss, bulkDismiss }
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/hooks/useJobActions.ts
git commit -m "feat(frontend): add useJobActions hook with React Query mutations"
```

---

## Task 6: Create `features/jobs/components/DismissModal.tsx`

Extracted directly from `JobsPage.tsx`. No logic changes.

**Files:**
- Create: `frontend/src/features/jobs/components/DismissModal.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/DismissModal.tsx`**

```tsx
import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils'

const DISMISS_REASON_OPTIONS = [
  { value: 'user_dismissed',    label: 'Not relevant' },
  { value: 'user_not_relevant', label: 'Too junior / senior' },
  { value: 'user_not_relevant', label: 'Bad location' },
  { value: 'user_not_relevant', label: 'Compensation too low' },
  { value: 'user_dismissed',    label: 'Company not a fit' },
  { value: 'user_dismissed',    label: 'Other' },
]

interface DismissModalProps {
  open: boolean
  jobTitle: string
  onConfirm: (reason: string) => void
  onCancel: () => void
}

export function DismissModal({ open, jobTitle, onConfirm, onCancel }: DismissModalProps) {
  const [selectedIdx, setSelectedIdx] = useState(0)

  return (
    <Dialog.Root open={open} onOpenChange={v => !v && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-[#13131e] border border-white/[0.07] rounded-xl p-5 shadow-2xl">
          <Dialog.Title className="text-sm font-medium text-white mb-1">
            Dismiss job
          </Dialog.Title>
          <p className="text-xs text-slate-500 mb-4 truncate">"{jobTitle}"</p>
          <div className="space-y-2 mb-5">
            {DISMISS_REASON_OPTIONS.map((opt, i) => (
              <button
                key={i}
                onClick={() => setSelectedIdx(i)}
                className={cn(
                  'w-full text-left px-3 py-2.5 rounded-lg text-sm transition-colors border',
                  selectedIdx === i
                    ? 'bg-violet-600/20 border-violet-500/40 text-violet-300'
                    : 'bg-white/[0.03] border-white/[0.07] text-slate-300 hover:bg-white/[0.06]',
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <div className="flex gap-2">
            <button
              onClick={onCancel}
              className="flex-1 px-4 py-2 text-sm text-slate-400 bg-white/5 border border-white/10 rounded-lg hover:bg-white/[0.08] transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => onConfirm(DISMISS_REASON_OPTIONS[selectedIdx].value)}
              className="flex-1 px-4 py-2 text-sm font-medium text-white bg-red-600/80 hover:bg-red-600 rounded-lg transition-colors"
            >
              Dismiss
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/DismissModal.tsx
git commit -m "feat(frontend): extract DismissModal into features/jobs/components"
```

---

## Task 7: Create `features/jobs/components/JobStatusTabs.tsx`

Extracted status tabs. Shows the active tab's server-side total as the count badge.

**Files:**
- Create: `frontend/src/features/jobs/components/JobStatusTabs.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/JobStatusTabs.tsx`**

```tsx
import { cn } from '@/lib/utils'
import type { TabKey } from '@/features/jobs/hooks/useJobs'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'new',     label: 'New' },
  { key: 'saved',   label: 'Saved' },
  { key: 'applied', label: 'Applied' },
]

interface JobStatusTabsProps {
  tab: TabKey
  total: number
  onChange: (tab: TabKey) => void
}

export function JobStatusTabs({ tab, total, onChange }: JobStatusTabsProps) {
  return (
    <div className="flex items-center gap-0.5 px-6 pt-3 border-b border-white/[0.07]">
      {TABS.map(t => (
        <button
          key={t.key}
          onClick={() => onChange(t.key)}
          className={cn(
            'px-4 py-2 text-sm font-medium transition-colors rounded-t-lg border-b-2 -mb-px',
            tab === t.key
              ? 'text-violet-300 border-violet-500 bg-violet-500/5'
              : 'text-slate-400 border-transparent hover:text-white hover:bg-white/[0.03]',
          )}
        >
          {t.label}
          {tab === t.key && total > 0 && (
            <span className="ml-1.5 text-xs px-1.5 py-0.5 rounded-full bg-violet-500/20 text-violet-300">
              {total}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/JobStatusTabs.tsx
git commit -m "feat(frontend): extract JobStatusTabs into features/jobs/components"
```

---

## Task 8: Create `features/jobs/components/JobFilters.tsx`

Extracts the filter bar and `CompanyMultiSelect` from `JobsPage.tsx`.

**Files:**
- Create: `frontend/src/features/jobs/components/JobFilters.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/JobFilters.tsx`**

```tsx
import { useState, useEffect, useRef } from 'react'
import { ChevronDown, Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DateFilter } from '@/features/jobs/hooks/useJobs'

// ── CompanyMultiSelect ────────────────────────────────────────────────────────

interface CompanyMultiSelectProps {
  companies: string[]
  selected: string[]
  onChange: (v: string[]) => void
}

function CompanyMultiSelect({ companies, selected, onChange }: CompanyMultiSelectProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const visible = companies.filter(c =>
    c.toLowerCase().includes(search.toLowerCase())
  )

  function toggle(c: string) {
    onChange(selected.includes(c) ? selected.filter(v => v !== c) : [...selected, c])
  }

  const label =
    selected.length === 0
      ? 'All companies'
      : selected.length === 1
        ? selected[0]
        : `${selected.length} companies`

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className={cn(
          'flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border transition-colors min-w-[120px]',
          selected.length > 0
            ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
            : 'bg-white/5 border-white/10 text-slate-300 hover:bg-white/[0.08]',
        )}
      >
        <span className="flex-1 text-left truncate max-w-[140px]">{label}</span>
        <ChevronDown size={11} className="shrink-0 text-slate-500" />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 w-56 bg-[#13131e] border border-white/[0.1] rounded-lg shadow-xl z-20 overflow-hidden">
          <div className="p-2 border-b border-white/[0.07]">
            <input
              autoFocus
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search…"
              className="w-full bg-white/5 border border-white/10 rounded-md px-2.5 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-violet-500"
            />
          </div>
          <div className="max-h-52 overflow-y-auto py-1">
            {visible.length === 0 ? (
              <p className="text-xs text-slate-500 px-3 py-2">No results</p>
            ) : (
              visible.map(c => (
                <button
                  key={c}
                  onClick={() => toggle(c)}
                  className={cn(
                    'w-full text-left px-3 py-2 text-xs transition-colors flex items-center gap-2',
                    selected.includes(c)
                      ? 'text-violet-300 bg-violet-500/10'
                      : 'text-slate-300 hover:bg-white/[0.05]',
                  )}
                >
                  <div className={cn(
                    'w-3.5 h-3.5 rounded border flex items-center justify-center shrink-0',
                    selected.includes(c) ? 'bg-violet-500 border-violet-500' : 'border-white/20',
                  )}>
                    {selected.includes(c) && <Check size={9} className="text-white" />}
                  </div>
                  {c}
                </button>
              ))
            )}
          </div>
          {selected.length > 0 && (
            <div className="border-t border-white/[0.07] p-2">
              <button
                onClick={() => { onChange([]); setSearch('') }}
                className="w-full text-xs text-slate-500 hover:text-slate-300 transition-colors py-1"
              >
                Clear selection
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── FilterPill ─────────────────────────────────────────────────────────────────

function FilterPill({
  active,
  color = 'violet',
  onClick,
  children,
}: {
  active: boolean
  color?: 'violet' | 'emerald'
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
        active && color === 'violet' && 'bg-violet-500/15 border-violet-500/30 text-violet-300',
        active && color === 'emerald' && 'bg-emerald-500/15 border-emerald-500/30 text-emerald-300',
        !active && 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]',
      )}
    >
      {children}
    </button>
  )
}

function Divider() {
  return <div className="w-px h-4 bg-white/[0.08] mx-0.5" />
}

// ── JobFilters ─────────────────────────────────────────────────────────────────

interface JobFiltersProps {
  companies: string[]
  selectedCompanies: string[]
  onCompaniesChange: (v: string[]) => void
  remoteFilter: boolean
  onRemoteChange: (v: boolean) => void
  expFilter: string[]
  onExpChange: (v: string[]) => void
  dateFilter: DateFilter
  onDateChange: (v: DateFilter) => void
  sponsorshipFilter: boolean
  onSponsorshipChange: (v: boolean) => void
  noSponsorshipFilter: boolean
  onNoSponsorshipChange: (v: boolean) => void
  noRequirementsFilter: boolean
  onNoRequirementsChange: (v: boolean) => void
  citizenshipReqFilter: boolean
  onCitizenshipReqChange: (v: boolean) => void
  employTypes: string[]
  onEmployTypesChange: (types: string[]) => void
}

const EXP_OPTIONS = ['0-2', '3-5', '5 <'] as const
const DATE_OPTIONS = ['all', '30d', '7d'] as const
const EMPLOY_TYPE_OPTIONS = [{ value: 'full_time', label: 'Full-time' }]

export function JobFilters({
  companies, selectedCompanies, onCompaniesChange,
  remoteFilter, onRemoteChange,
  expFilter, onExpChange,
  dateFilter, onDateChange,
  sponsorshipFilter, onSponsorshipChange,
  noSponsorshipFilter, onNoSponsorshipChange,
  noRequirementsFilter, onNoRequirementsChange,
  citizenshipReqFilter, onCitizenshipReqChange,
  employTypes, onEmployTypesChange,
}: JobFiltersProps) {
  function toggleExp(opt: string) {
    onExpChange(expFilter.includes(opt) ? expFilter.filter(v => v !== opt) : [...expFilter, opt])
  }

  function toggleEmployType(val: string) {
    onEmployTypesChange(
      employTypes.includes(val) ? employTypes.filter(v => v !== val) : [...employTypes, val]
    )
  }

  return (
    <div className="flex items-center gap-2 px-6 py-2.5 border-b border-white/[0.07] flex-wrap">
      <CompanyMultiSelect
        companies={companies}
        selected={selectedCompanies}
        onChange={onCompaniesChange}
      />

      <Divider />

      {EXP_OPTIONS.map(opt => (
        <FilterPill key={opt} active={expFilter.includes(opt)} onClick={() => toggleExp(opt)}>
          {opt === '5 <' ? '5+ yrs' : `${opt} yrs`}
        </FilterPill>
      ))}

      <Divider />

      {DATE_OPTIONS.map(d => (
        <FilterPill key={d} active={dateFilter === d} onClick={() => onDateChange(d as DateFilter)}>
          {d === 'all' ? 'All' : d}
        </FilterPill>
      ))}

      <Divider />

      {EMPLOY_TYPE_OPTIONS.map(opt => (
        <FilterPill
          key={opt.value}
          active={employTypes.includes(opt.value)}
          onClick={() => toggleEmployType(opt.value)}
        >
          {opt.label}
        </FilterPill>
      ))}

      <Divider />

      <FilterPill color="emerald" active={remoteFilter} onClick={() => onRemoteChange(!remoteFilter)}>
        Remote
      </FilterPill>

      <FilterPill active={sponsorshipFilter} onClick={() => onSponsorshipChange(!sponsorshipFilter)}>
        Sponsorship
      </FilterPill>

      <FilterPill active={noSponsorshipFilter} onClick={() => onNoSponsorshipChange(!noSponsorshipFilter)}>
        No Sponsorship
      </FilterPill>

      <FilterPill active={noRequirementsFilter} onClick={() => onNoRequirementsChange(!noRequirementsFilter)}>
        No Citizenship Req.
      </FilterPill>

      <FilterPill active={citizenshipReqFilter} onClick={() => onCitizenshipReqChange(!citizenshipReqFilter)}>
        Citizenship Req.
      </FilterPill>
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/JobFilters.tsx
git commit -m "feat(frontend): extract JobFilters and CompanyMultiSelect into features/jobs/components"
```

---

## Task 9: Create `features/jobs/components/jobs-columns.tsx`

Moves column definitions from `pages/jobs-columns.tsx` into the feature folder. Updates `CompanyAvatar` and `formatSalary` imports to use the new shared/services locations.

**Files:**
- Create: `frontend/src/features/jobs/components/jobs-columns.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/jobs-columns.tsx`**

```tsx
import { useRef, useEffect } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { MapPin, Wifi } from 'lucide-react'
import type { Job } from '@/types'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { formatRelativeTime, cn } from '@/lib/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { formatSalary } from '@/services/jobs.service'

export interface JobTableActions {
  selectedIds: Set<string>
  filteredIds: string[]
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
}

function IndeterminateCheckbox({
  checked,
  indeterminate,
  onChange,
}: {
  checked: boolean
  indeterminate: boolean
  onChange: (checked: boolean) => void
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate
  }, [indeterminate])
  return (
    <input
      ref={ref}
      type="checkbox"
      checked={checked}
      onChange={e => onChange(e.target.checked)}
      className="accent-violet-500 cursor-pointer"
    />
  )
}

export function buildJobColumns(actions: JobTableActions): ColumnDef<Job, unknown>[] {
  const { selectedIds, filteredIds, onToggleSelect, onSelectAll } = actions
  const allSelected = filteredIds.length > 0 && filteredIds.every(id => selectedIds.has(id))
  const someSelected = filteredIds.some(id => selectedIds.has(id))

  return [
    {
      id: 'select',
      size: 40,
      enableSorting: false,
      header: () => (
        <IndeterminateCheckbox
          checked={allSelected}
          indeterminate={someSelected && !allSelected}
          onChange={onSelectAll}
        />
      ),
      cell: ({ row }) => (
        <div onClick={e => e.stopPropagation()}>
          <input
            type="checkbox"
            checked={selectedIds.has(row.original.id)}
            onChange={e => onToggleSelect(row.original.id, e.target.checked)}
            className="accent-violet-500 cursor-pointer"
          />
        </div>
      ),
    },
    {
      id: 'company',
      size: 180,
      accessorFn: row => row.company?.name ?? row.company_id ?? '',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Company" />,
      cell: ({ row }) => {
        const name = row.original.company?.name ?? row.original.company_id ?? ''
        return (
          <div className="flex items-center gap-2">
            <CompanyAvatar name={name} size="sm" />
            <span className="text-xs text-slate-300 truncate max-w-[120px]">{name}</span>
          </div>
        )
      },
    },
    {
      accessorKey: 'title',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
      cell: ({ getValue }) => (
        <span className="font-medium text-white text-sm">{getValue() as string}</span>
      ),
    },
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-slate-400">Location</span>,
      cell: ({ row }) => {
        const j = row.original
        if (j.remote)
          return (
            <span className="inline-flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 rounded-md">
              <Wifi size={10} /> Remote
            </span>
          )
        if (j.location)
          return (
            <span className="inline-flex items-center gap-1 text-xs text-slate-400">
              <MapPin size={10} /> {j.location}
            </span>
          )
        return <span className="text-xs text-slate-600">—</span>
      },
    },
    {
      id: 'salary',
      size: 140,
      enableSorting: false,
      header: () => <span className="text-slate-400">Salary</span>,
      cell: ({ row }) => {
        const salary = formatSalary(row.original.salary_range)
        return salary
          ? <span className="text-xs text-amber-300">{salary}</span>
          : <span className="text-xs text-slate-600">—</span>
      },
    },
    {
      accessorKey: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Added" />,
      cell: ({ getValue }) => (
        <span className="text-xs text-slate-500">
          {formatRelativeTime(getValue() as string)}
        </span>
      ),
    },
  ]
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/jobs-columns.tsx
git commit -m "feat(frontend): move job column definitions into features/jobs/components"
```

---

## Task 10: Create `features/jobs/components/JobsTable.tsx`

Wraps `DataTable` with job-specific column building and selection logic.

**Files:**
- Create: `frontend/src/features/jobs/components/JobsTable.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/JobsTable.tsx`**

```tsx
import { useMemo } from 'react'
import { DataTable } from '@/components/ui/data-table'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { buildJobColumns } from '@/features/jobs/components/jobs-columns'

interface JobsTableProps {
  jobs: Job[]
  total: number
  loading: boolean
  tab: TabKey
  page: number
  limit: number
  selectedIds: Set<string>
  activeJobId: string | null
  onRowClick: (job: Job) => void
  onToggleSelect: (id: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
  onPageChange: (page: number) => void
  onLimitChange: (limit: number) => void
}

const EMPTY_MESSAGES: Record<TabKey, string> = {
  new:     'No new jobs — run a scan to populate the feed',
  saved:   'No saved jobs yet',
  applied: 'No applied jobs yet',
}

export function JobsTable({
  jobs, total, loading, tab,
  page, limit,
  selectedIds, activeJobId,
  onRowClick, onToggleSelect, onSelectAll,
  onPageChange, onLimitChange,
}: JobsTableProps) {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const columns = useMemo(() => buildJobColumns({
    selectedIds,
    filteredIds: jobs.map(j => j.id),
    onToggleSelect,
    onSelectAll,
  }), [selectedIds, jobs])

  return (
    <div className="flex-1 overflow-auto min-w-0">
      <DataTable
        columns={columns}
        data={jobs}
        loading={loading}
        emptyMessage={EMPTY_MESSAGES[tab]}
        getRowId={j => j.id}
        onRowClick={onRowClick}
        getRowClassName={job => {
          if (selectedIds.has(job.id)) return 'bg-violet-500/[0.06]'
          if (activeJobId === job.id)  return 'bg-violet-500/[0.08]'
          return 'hover:bg-white/[0.03]'
        }}
        serverPagination={{
          page,
          limit,
          total,
          onPageChange,
          onLimitChange,
        }}
      />
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/JobsTable.tsx
git commit -m "feat(frontend): extract JobsTable component into features/jobs/components"
```

---

## Task 11: Create `features/jobs/components/JobDetailPanel.tsx`

Extracted from `JobDetailDrawer` in `JobsPage.tsx`. Renamed to Panel to match the new side-panel design. No logic changes — parent controls visibility and width.

**Files:**
- Create: `frontend/src/features/jobs/components/JobDetailPanel.tsx`

- [ ] **Step 1: Create `src/features/jobs/components/JobDetailPanel.tsx`**

```tsx
import { X, ExternalLink, MapPin, Wifi, Briefcase, DollarSign, Bookmark, BookmarkCheck, Send, Trash2 } from 'lucide-react'
import type { Job } from '@/types'
import { formatRelativeTime, cn } from '@/lib/utils'
import { formatSalary } from '@/services/jobs.service'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'

interface JobDetailPanelProps {
  job: Job
  onClose: () => void
  onSave: (job: Job) => void
  onApply: (job: Job) => void
  onDismiss: (job: Job) => void
}

export function JobDetailPanel({ job, onClose, onSave, onApply, onDismiss }: JobDetailPanelProps) {
  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id ?? ''

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-[#0d0d14]">
      {/* Header */}
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        <div className="flex items-start justify-between gap-3 mb-3">
          <div className="flex items-start gap-3 min-w-0">
            <CompanyAvatar name={companyName} size="md" />
            <div className="min-w-0">
              <p className="text-base font-semibold text-white leading-tight">{job.title}</p>
              <p className="text-sm text-slate-400 mt-0.5">{companyName}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="shrink-0 p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
          >
            <X size={15} />
          </button>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1.5 text-xs text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>
          {!job.saved && job.status !== 'applied' && (
            <button
              onClick={() => onSave(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-violet-600 hover:bg-violet-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Bookmark size={12} /> Save
            </button>
          )}
          {job.saved && job.status !== 'applied' && (
            <button
              onClick={() => onSave(job)}
              className="flex items-center gap-1.5 text-xs text-violet-300 bg-violet-500/10 border border-violet-500/20 hover:bg-violet-500/20 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <BookmarkCheck size={12} /> Saved
            </button>
          )}
          {job.status !== 'applied' && (
            <button
              onClick={() => onApply(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-emerald-600 hover:bg-emerald-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Send size={12} /> Mark applied
            </button>
          )}
          {job.status !== 'applied' && (
            <button
              onClick={() => onDismiss(job)}
              className="ml-auto p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
              title="Dismiss"
            >
              <Trash2 size={13} />
            </button>
          )}
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {/* Meta badges */}
        <div className="flex flex-wrap gap-2">
          {job.location && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <MapPin size={11} /> {job.location}
            </span>
          )}
          {job.remote && (
            <span className="flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 rounded-lg">
              <Wifi size={11} /> Remote
            </span>
          )}
          {salary && (
            <span className="flex items-center gap-1 text-xs text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              <DollarSign size={11} /> {salary}
            </span>
          )}
          {job.employ_type && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <Briefcase size={11} /> {job.employ_type.replace('_', ' ')}
            </span>
          )}
          {job.experience_years != null && (
            <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              {job.experience_years}+ yrs exp
            </span>
          )}
        </div>

        {/* Flag indicators */}
        <div className="flex flex-wrap gap-2">
          <span className={cn(
            'text-xs px-2.5 py-1 rounded-lg border',
            job.sponsorship
              ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
              : 'text-slate-500 bg-white/[0.03] border-white/[0.06]',
          )}>
            {job.sponsorship ? '✓ Sponsorship' : '✗ No sponsorship'}
          </span>
          {job.requires_citizen && (
            <span className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              Citizenship required
            </span>
          )}
          {job.security_clearance && (
            <span className="text-xs text-red-400 bg-red-500/10 border border-red-500/20 px-2.5 py-1 rounded-lg">
              Security clearance required
            </span>
          )}
        </div>

        <p className="text-xs text-slate-500">Added {formatRelativeTime(job.added_at)}</p>

        {(job.role_brief || job.llm_summary) && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Summary</h3>
            <p className="text-sm text-slate-300 leading-relaxed">{job.role_brief ?? job.llm_summary}</p>
          </div>
        )}

        {job.responsibilities && job.responsibilities.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Responsibilities</h3>
            <ul className="list-disc list-inside space-y-1">
              {job.responsibilities.map((r, i) => (
                <li key={i} className="text-sm text-slate-300">{r}</li>
              ))}
            </ul>
          </div>
        )}

        {techStack.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Tech stack</h3>
            <div className="flex flex-wrap gap-1.5">
              {techStack.map(t => (
                <span key={t} className="text-xs bg-white/[0.07] text-slate-300 px-2 py-0.5 rounded-md font-mono">
                  {t}
                </span>
              ))}
            </div>
          </div>
        )}

        {job.notes && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Notes</h3>
            <p className="text-sm text-slate-300 leading-relaxed">{job.notes}</p>
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/components/JobDetailPanel.tsx
git commit -m "feat(frontend): extract JobDetailPanel into features/jobs/components"
```

---

## Task 12: Create `features/jobs/index.ts`

Barrel export so pages import from `@/features/jobs` not from deep paths.

**Files:**
- Create: `frontend/src/features/jobs/index.ts`

- [ ] **Step 1: Create `src/features/jobs/index.ts`**

```ts
export { useJobs }        from '@/features/jobs/hooks/useJobs'
export { useJobActions }  from '@/features/jobs/hooks/useJobActions'
export { JobsTable }      from '@/features/jobs/components/JobsTable'
export { JobDetailPanel } from '@/features/jobs/components/JobDetailPanel'
export { JobFilters }     from '@/features/jobs/components/JobFilters'
export { JobStatusTabs }  from '@/features/jobs/components/JobStatusTabs'
export { DismissModal }   from '@/features/jobs/components/DismissModal'
export type { TabKey, DateFilter } from '@/features/jobs/hooks/useJobs'
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/features/jobs/index.ts
git commit -m "feat(frontend): add features/jobs barrel export"
```

---

## Task 13: Rewrite `pages/JobsPage.tsx` as thin shell

Replace the entire file. All state lives in hooks; the page just composes components.

**Files:**
- Modify: `frontend/src/pages/JobsPage.tsx` (full rewrite)

- [ ] **Step 1: Rewrite `src/pages/JobsPage.tsx`**

```tsx
import { useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import type { Job } from '@/types'
import {
  useJobs, useJobActions,
  JobStatusTabs, JobFilters, JobsTable,
  JobDetailPanel, DismissModal,
} from '@/features/jobs'

export function JobsPage() {
  const jobs    = useJobs()
  const actions = useJobActions()

  const [activeJob, setActiveJob]         = useState<Job | null>(null)
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [selectedIds, setSelectedIds]     = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth]       = useState(440)

  function startDrag(e: React.MouseEvent) {
    e.preventDefault()
    const startX     = e.clientX
    const startWidth = panelWidth
    function onMove(e: MouseEvent) {
      setPanelWidth(Math.min(Math.max(startWidth + (startX - e.clientX), 300), 720))
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  function handleSave(job: Job) {
    actions.save.mutate({ id: job.id, saved: !job.saved })
  }

  function handleApply(job: Job) {
    actions.apply.mutate(job.id)
    if (activeJob?.id === job.id) setActiveJob(null)
  }

  function handleDismissConfirm(reason: string) {
    if (!dismissTarget) return
    actions.dismiss.mutate({ id: dismissTarget.id, reason })
    if (activeJob?.id === dismissTarget.id) setActiveJob(null)
    setDismissTarget(null)
  }

  function handleBulkDismiss() {
    const ids = Array.from(selectedIds)
    actions.bulkDismiss.mutate(ids)
    if (activeJob && selectedIds.has(activeJob.id)) setActiveJob(null)
    setSelectedIds(new Set())
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Jobs</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {jobs.total} job{jobs.total !== 1 ? 's' : ''}
          </p>
        </div>
        <button
          onClick={() => jobs.refetch()}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
          title="Refresh"
        >
          <RefreshCw size={14} className={jobs.isLoading ? 'animate-spin' : ''} />
        </button>
      </div>

      <JobStatusTabs
        tab={jobs.tab}
        total={jobs.total}
        onChange={t => { jobs.setTab(t); jobs.setPage(1) }}
      />

      <JobFilters
        companies={jobs.companies}
        selectedCompanies={jobs.selectedCompanies}
        onCompaniesChange={jobs.setSelectedCompanies}
        remoteFilter={jobs.remoteFilter}
        onRemoteChange={jobs.setRemoteFilter}
        expFilter={jobs.expFilter}
        onExpChange={jobs.setExpFilter}
        dateFilter={jobs.dateFilter}
        onDateChange={jobs.setDateFilter}
        sponsorshipFilter={jobs.sponsorshipFilter}
        onSponsorshipChange={jobs.setSponsorshipFilter}
        noSponsorshipFilter={jobs.noSponsorshipFilter}
        onNoSponsorshipChange={jobs.setNoSponsorshipFilter}
        noRequirementsFilter={jobs.noRequirementsFilter}
        onNoRequirementsChange={jobs.setNoRequirementsFilter}
        citizenshipReqFilter={jobs.citizenshipReqFilter}
        onCitizenshipReqChange={jobs.setCitizenshipReqFilter}
        employTypes={jobs.employTypes}
        onEmployTypesChange={jobs.setEmployTypes}
      />

      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 px-6 py-2 bg-violet-500/10 border-b border-violet-500/20">
          <span className="text-xs text-violet-300 font-medium">
            {selectedIds.size} selected
          </span>
          <button
            onClick={handleBulkDismiss}
            className="flex items-center gap-1.5 text-xs text-white bg-red-600/80 hover:bg-red-600 px-3 py-1.5 rounded-lg transition-colors"
          >
            <Trash2 size={12} /> Mark not relevant
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            className="text-xs text-slate-400 hover:text-slate-200 transition-colors ml-auto"
          >
            Clear
          </button>
        </div>
      )}

      {/* Content: table + optional detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <JobsTable
          jobs={jobs.jobs}
          total={jobs.total}
          loading={jobs.isLoading}
          tab={jobs.tab}
          page={jobs.page}
          limit={jobs.limit}
          selectedIds={selectedIds}
          activeJobId={activeJob?.id ?? null}
          onRowClick={setActiveJob}
          onToggleSelect={(id, checked) => {
            setSelectedIds(prev => {
              const n = new Set(prev)
              checked ? n.add(id) : n.delete(id)
              return n
            })
          }}
          onSelectAll={checked => {
            setSelectedIds(checked ? new Set(jobs.jobs.map(j => j.id)) : new Set())
          }}
          onPageChange={jobs.setPage}
          onLimitChange={l => { jobs.setLimit(l); jobs.setPage(1) }}
        />

        {activeJob && (
          <>
            <div
              onMouseDown={startDrag}
              className="w-1 shrink-0 cursor-col-resize bg-white/[0.06] hover:bg-violet-500/50 transition-colors"
            />
            <div
              style={{ width: panelWidth }}
              className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden"
            >
              <JobDetailPanel
                job={activeJob}
                onClose={() => setActiveJob(null)}
                onSave={handleSave}
                onApply={handleApply}
                onDismiss={job => setDismissTarget(job)}
              />
            </div>
          </>
        )}
      </div>

      <DismissModal
        open={!!dismissTarget}
        jobTitle={dismissTarget?.title ?? ''}
        onConfirm={handleDismissConfirm}
        onCancel={() => setDismissTarget(null)}
      />
    </div>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd frontend && git add src/pages/JobsPage.tsx
git commit -m "feat(frontend): rewrite JobsPage as thin shell composing features/jobs"
```

---

## Task 14: Delete `pages/jobs-columns.tsx` + verify build

The column definitions now live in `features/jobs/components/jobs-columns.tsx`. The old file is dead code.

**Files:**
- Delete: `frontend/src/pages/jobs-columns.tsx`

- [ ] **Step 1: Delete the old columns file**

```bash
rm frontend/src/pages/jobs-columns.tsx
```

- [ ] **Step 2: Confirm nothing imports it**

```bash
cd frontend && grep -r "pages/jobs-columns" src/
```

Expected: no output (no remaining imports).

- [ ] **Step 3: Full type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Run the dev server and verify the Jobs page loads**

```bash
cd frontend && npm run dev
```

Open `http://localhost:5173`, navigate to `/jobs`. Verify:
- Jobs table loads with data
- Tab switching works (New / Saved / Applied)
- Filters are interactive (pills toggle, company dropdown opens)
- Clicking a row opens the detail panel
- Drag handle resizes the panel
- Save / Apply / Dismiss buttons work and list refreshes automatically

- [ ] **Step 5: Commit**

```bash
cd frontend && git add -A
git commit -m "feat(frontend): delete dead pages/jobs-columns.tsx — moved to features/jobs"
```

---

## Done

After Task 14, the architecture is:

```
src/
├── api/                          ← HTTP layer (unchanged)
├── services/
│   └── jobs.service.ts           ← formatSalary, avatarColor (single source)
├── features/
│   └── jobs/
│       ├── components/
│       │   ├── DismissModal.tsx
│       │   ├── JobDetailPanel.tsx
│       │   ├── JobFilters.tsx
│       │   ├── JobStatusTabs.tsx
│       │   ├── JobsTable.tsx
│       │   └── jobs-columns.tsx
│       ├── hooks/
│       │   ├── useJobs.ts         ← React Query + all filter state
│       │   └── useJobActions.ts   ← React Query mutations
│       └── index.ts
├── components/
│   ├── ui/                       ← unchanged
│   └── shared/
│       └── CompanyAvatar.tsx     ← deduplicated
├── lib/
│   ├── utils.ts                  ← unchanged
│   └── queryClient.ts            ← new
└── pages/
    └── JobsPage.tsx              ← thin shell (~100 lines)
```
