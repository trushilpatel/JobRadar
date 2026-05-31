# Console Frontend — Architecture Reference

**Last updated:** 2026-05-28
**Full design spec:** `docs/superpowers/specs/2026-05-28-frontend-architecture-design.md`
**Status:** Complete — boundary rules enforced as CI errors as of 2026-05-29

---

## The rule (one sentence)

Imports flow **downward only** — a module at layer N may only import from layers 0 through N−1.
The linter enforces this. A violation is a CI error.

---

## Layers (low → high)

```
0  config/      env, routes, app-wide constants        imports nothing from src/
1  lib/          framework infra, hooks, utils          no feature / page / app imports
2  components/   ui/ (zero domain)  shared/ (light)    no feature / page / app imports
3  features/     domain slices, self-contained          NO cross-feature imports
4  pages/        route-level shells, compose features   no app/ imports
5  app/          router, providers, bootstrap           composes everything
```

---

## Canonical folder tree

```
src/
├── app/                          # layer 5
│   ├── App.tsx
│   ├── providers.tsx             # QueryClientProvider, NuqsAdapter, Toaster
│   ├── router.tsx                # all <Route> definitions
│   └── auth-bootstrap.tsx
│
├── pages/                        # layer 4 — route shells, no data fetching
│   ├── DashboardPage.tsx
│   ├── JobsPage.tsx
│   ├── InsightsPage.tsx
│   ├── SettingsPage.tsx
│   ├── AccountPage.tsx
│   ├── CompaniesPage.tsx
│   └── admin/
│       ├── AdminUsersPage.tsx
│       ├── AdminAuditPage.tsx
│       ├── ResumeLibraryPage.tsx
│       ├── ResumeDetailPage.tsx
│       ├── ResumeEditorPage.tsx
│       └── ApplyQueuePage.tsx
│
├── features/                     # layer 3 — one folder per domain
│   ├── jobs/
│   │   ├── api/
│   │   │   └── jobs.api.ts       # fetch functions + zod schemas + z.infer types
│   │   ├── components/
│   │   ├── commands/
│   │   │   └── jobCommands.ts    # keyboard shortcut registry for jobs feature
│   │   ├── hooks/
│   │   ├── queryKeys.ts
│   │   └── index.ts              # ← PUBLIC API — the ONLY legal import path for consumers
│   │
│   ├── companies/                # same shape
│   │   ├── api/
│   │   │   ├── companies.api.ts
│   │   │   └── adminCompanies.api.ts
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── constants.ts          # constants used only within this feature
│   │   ├── queryKeys.ts
│   │   └── index.ts
│   │
│   ├── dashboard/
│   │   ├── api/
│   │   │   └── dashboard.api.ts
│   │   ├── components/           # all panels live here
│   │   ├── queryKeys.ts
│   │   └── index.ts
│   │
│   ├── insights/
│   │   ├── api/
│   │   │   └── insights.api.ts
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── queryKeys.ts
│   │   └── index.ts
│   │
│   ├── settings/
│   │   ├── api/
│   │   │   └── settings.api.ts
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── queryKeys.ts
│   │   └── index.ts
│   │
│   ├── auth/
│   │   ├── api/
│   │   │   ├── me.api.ts
│   │   │   ├── adminUsers.api.ts
│   │   │   └── adminAudit.api.ts
│   │   ├── components/           # LoginPage, AuthCallback, RequireAuth, RequireAdmin
│   │   ├── store/
│   │   │   └── auth.store.ts
│   │   └── index.ts
│   │
│   └── resumes/                  # deferred — same shape when resumed
│       ├── components/
│       ├── hooks/
│       ├── types.ts              # domain-local types (flat file, not a subdir)
│       ├── queryKeys.ts
│       └── index.ts
│
├── components/                   # layer 2 — zero domain knowledge
│   ├── ui/                       # Radix-based primitives, Tailwind-composed
│   │   ├── button.tsx
│   │   ├── dialog.tsx
│   │   ├── data-table.tsx
│   │   └── ... (20+ files)
│   └── shared/                   # lightly domain-aware, used by 2+ features
│       ├── CompanyAvatar.tsx
│       └── ErrorBoundary.tsx
│
├── lib/                          # layer 1 — framework infrastructure
│   ├── core/
│   │   ├── api-client.ts         # HTTP client with auth, retry, error handling
│   │   ├── api-error.ts          # ApiValidationError class
│   │   ├── supabase.ts           # Supabase client singleton
│   │   ├── queryClient.ts        # TanStack Query client + defaults
│   │   └── session-ref.ts        # session ref for non-React code
│   ├── hooks/
│   │   ├── use-debounced-value.ts
│   │   ├── use-media-query.ts
│   │   ├── use-undo-toast.ts
│   │   └── hidden-items.ts
│   ├── commands/                 # command registry — keyboard shortcuts, dispatch, platform detection
│   │   ├── types.ts
│   │   ├── buildDispatch.ts
│   │   ├── formatKeys.ts
│   │   ├── platform.ts
│   │   └── useCommandKeyboard.ts
│   └── utils/
│       └── utils.ts              # cn(), formatRelativeTime(), avatarColor(), formatSalary(), etc.
│
├── config/                       # layer 0 — imports nothing from src/
│   ├── routes.ts                 # ALL route builders (was lib/routes.ts)
│   ├── env.ts                    # typed import.meta.env (VITE_* vars)
│   ├── constants/
│   │   └── jobStatus.ts          # JOB_STATUS, TAB_KEYS, etc.
│   └── copy/
│       └── index.ts              # UI copy strings — nav labels, status copy, feature text
│
└── types/                        # shared types used across 2+ features
    └── index.ts                  # shrinks as domain types move to features/<x>/types/
```

---

## The six import rules

Enforced via `no-restricted-imports` rules in `eslint.config.js`. (`eslint-plugin-boundaries` is installed but the active rules use the built-in `no-restricted-imports` so `@/` path aliases are matched as-written without a resolver.) These are **CI errors**, not warnings.

| Rule                                                            | What it prevents                     |
| --------------------------------------------------------------- | ------------------------------------ |
| `config` imports nothing from `src/`                            | Config is pure; no circular risk     |
| `lib` imports nothing from `features/`, `pages/`, `app/`        | Infra stays infrastructure           |
| `components` imports nothing from `features/`, `pages/`, `app/` | Shared UI stays domain-free          |
| `features/<X>` imports nothing from `features/<Y>`              | No hidden cross-feature coupling     |
| External code imports from `features/<X>/index.ts` only         | Internal details are private         |
| `pages/` imports nothing from `app/`                            | Pages don't know about app bootstrap |

---

## Per-feature public API (index.ts contract)

Every feature exports **only** through its `index.ts`. Nothing outside the feature folder may import
from a deeper path like `@/features/jobs/hooks/useJobs`. The barrel is the contract.

```ts
// features/jobs/index.ts — example
export { JobsTable } from './components/JobsTable'
export { JobDetailPanel } from './components/JobDetailPanel'
export { useJobs } from './hooks/useJobs'
export { jobsUrl } from './hooks/useJobUrlState'
export type { Job } from './api/jobs.api'
```

If you need to expose something new to another layer, you add it to `index.ts` — an explicit,
reviewable, diff-visible act.

---

## Where things go — quick reference

| Thing                                 | Goes in                                                       |
| ------------------------------------- | ------------------------------------------------------------- |
| New route builder                     | `config/routes.ts`                                            |
| New app-wide constant                 | `config/constants/<name>.ts`                                  |
| New HTTP utility                      | `lib/core/api-client.ts` or new file in `lib/core/`           |
| New shared hook (used by 2+ features) | `lib/hooks/`                                                  |
| New formatting utility                | `lib/utils/utils.ts`                                          |
| New UI primitive (Radix / Tailwind)   | `components/ui/`                                              |
| New component used by 2+ features     | `components/shared/`                                          |
| New domain component / hook           | `features/<domain>/components/` or `features/<domain>/hooks/` |
| New API function                      | `features/<domain>/api/<domain>.api.ts`                       |
| New domain-local type                 | `features/<domain>/types.ts` (flat file, not a subdir)        |
| New type used by 2+ features          | `types/index.ts`                                              |
| New page (route)                      | `pages/` (or `pages/admin/` for admin routes)                 |
| New route definition                  | `app/router.tsx`                                              |

---

## Starting a new feature

```bash
npm run scaffold feature <name>
```

Generates the canonical skeleton under `features/<name>/`:

- `api/<name>.api.ts` with a zod schema stub
- `components/.gitkeep`
- `hooks/.gitkeep`
- `types/index.ts`
- `queryKeys.ts`
- `index.ts` barrel

The generated skeleton passes lint. Do not hand-write feature structure.

---

## AI agent rules (per-layer CLAUDE.md)

Each layer directory contains a `CLAUDE.md` that auto-loads when an agent edits files there.
It states:

- Which layer this is and what it is for
- What may be imported (layers below only)
- What must NOT be imported
- Where to place files that don't belong here

These files are the machine-readable equivalent of this document — the linter enforces
structure, the CLAUDE.md files explain it.

---

## What is explicitly NOT here

- **`src/api/`** (top-level) — deleted; API functions live in `features/<domain>/api/`
- **`src/services/`** — deleted; utils moved to `lib/utils/`
- **`src/constants/`** — deleted; routes → `config/routes.ts`, jobStatus → `config/constants/`
- **`src/constants/routes.ts`** — deleted; `config/routes.ts` is the only route file
- **Cross-feature imports** — banned by linter; compose at page level instead
- **Deep feature imports** — banned by linter; use `index.ts` barrel only
