# Frontend Architecture — World-Class Design Spec

**Date:** 2026-05-28
**Status:** Approved — proceed to implementation plan
**Scope:** `ui/console/src/` — structure, enforced boundaries, AI-proof conventions

---

## 1. Problem statement

The current frontend has the right instincts (feature slices, TanStack Query, typed API layer) but
lacks two things that matter at scale:

1. **A machine-enforced layering rule.** The architecture exists only in CLAUDE.md and developer
   discipline. Nothing stops an illegal import — upward, cross-feature, or deep into a feature's
   internals. It silently drifts.

2. **A consistent, predictable shape.** Features have different internal layouts. `api/` is a
   top-level concern rather than owned by each feature. Two route files coexist. `services/` is a
   vestigial folder nobody can explain. `types/index.ts` is a 342-line dump. These inconsistencies
   mean no one — human or AI agent — can confidently answer "where does this go?"

The goal: every file has exactly one correct home, and an incorrect import is a CI error before
any human sees it.

---

## 2. Audit findings (the concrete warts)

| #   | Problem                               | Evidence                                                                                                                          |
| --- | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Two route systems                     | `lib/routes.ts` (typed builders, 54 importers) AND `constants/routes.ts` (raw string constants, diverging)                        |
| 2   | Vestigial `services/`                 | 1 file (`jobs.service.ts` — pure util), 3 importers, conceptually belongs in `lib/utils/`                                         |
| 3   | `types/index.ts` is a dump            | 342 lines, every type in the app — no domain ownership, grows forever                                                             |
| 4   | Upward import violation               | `components/Sidebar.tsx` imports `@/features/auth/store` (shared layer importing from feature layer)                              |
| 5   | Cross-feature import violation        | `features/jobs` imports directly into `features/resumes` internals — bypasses public API                                          |
| 6   | Inconsistent feature shapes           | `dashboard` dumps panels at root (no `components/`); `insights` has no `index.ts`; `auth` mixes pages, guards, store flat at root |
| 7   | `api/` is top-level, not domain-owned | 9 API files owned by no feature; domain logic is split from its consumers                                                         |
| 8   | Pages bypass feature barrels          | `InsightsPage` imports 7 components from feature internals, not via `index.ts`                                                    |
| 9   | `lib/` is a catch-all                 | Holds infra (`api-client`, `supabase`), hooks (`use-debounced-value`), utils — no internal structure                              |
| 10  | `App.tsx` does too much               | Routing, providers, and bootstrap all in one file                                                                                 |

---

## 3. Target architecture — 6-layer unidirectional model

```
┌─────────────────────────────────────────────────────────┐
│  5. app/      Routes, providers, auth bootstrap         │ composes everything
├─────────────────────────────────────────────────────────┤
│  4. pages/    Route-level shells, no data fetching      │ composes features
├─────────────────────────────────────────────────────────┤
│  3. features/ Domain slices — self-contained            │ NO cross-feature imports
├─────────────────────────────────────────────────────────┤
│  2. components/ ui/ (zero domain)  shared/ (light)      │ no feature imports
├─────────────────────────────────────────────────────────┤
│  1. lib/      core/ hooks/ utils/ — framework infra     │ no feature imports
├─────────────────────────────────────────────────────────┤
│  0. config/   env, constants, routes                    │ imports nothing internal
└─────────────────────────────────────────────────────────┘
```

**The iron rule:** imports flow **downward only**. A module at layer N may only import from
layers 0 through N−1. The linter enforces this — CI fails on any violation.

---

## 4. Target folder tree

```
src/
├── app/                          # layer 5
│   ├── App.tsx                   # entry — mounts providers + router
│   ├── providers.tsx             # QueryClientProvider, NuqsAdapter, Toaster
│   ├── router.tsx                # all <Route> definitions
│   └── auth-bootstrap.tsx        # AuthBootstrap component (was inline in App.tsx)
│
├── pages/                        # layer 4 — route shells only, compose features
│   ├── DashboardPage.tsx
│   ├── JobsPage.tsx
│   ├── InsightsPage.tsx
│   ├── SettingsPage.tsx
│   ├── AccountPage.tsx
│   └── admin/
│       ├── CompaniesPage.tsx
│       ├── AdminUsersPage.tsx
│       ├── AdminAuditPage.tsx
│       ├── ResumeLibraryPage.tsx
│       ├── ResumeDetailPage.tsx
│       ├── ResumeEditorPage.tsx
│       └── ApplyQueuePage.tsx
│
├── features/                     # layer 3 — one folder per domain
│   ├── <domain>/
│   │   ├── api/                  # fetch functions + zod schemas + inferred types
│   │   │   └── <domain>.api.ts   # was src/api/<domain>.ts
│   │   ├── components/           # all UI components for this domain
│   │   ├── hooks/                # all hooks for this domain
│   │   ├── store/                # Zustand slice (if domain has global UI state)
│   │   ├── types/                # domain-local types (not shared app-wide)
│   │   ├── constants/            # domain-local constants
│   │   ├── queryKeys.ts          # query key factory
│   │   └── index.ts              # ← PUBLIC API — only legal entry point for consumers
│   │
│   ├── jobs/                     # canonical shape
│   ├── companies/
│   ├── dashboard/
│   ├── insights/
│   ├── settings/
│   ├── auth/
│   │   ├── components/           # LoginPage, AuthCallback, RequireAuth, RequireAdmin
│   │   ├── store/
│   │   │   └── auth.store.ts
│   │   ├── api/
│   │   └── index.ts
│   └── resumes/                  # deferred for resume work phase
│
├── components/                   # layer 2 — zero domain knowledge
│   ├── ui/                       # Radix-based primitives, Tailwind-composed
│   │   └── (23 existing files, unchanged)
│   └── shared/                   # lightly domain-aware (CompanyAvatar, ErrorBoundary)
│
├── lib/                          # layer 1 — framework infrastructure
│   ├── core/                     # api-client.ts, api-error.ts, supabase.ts,
│   │   │                         #   queryClient.ts, session-ref.ts
│   ├── hooks/                    # use-debounced-value.ts, use-media-query.ts,
│   │   │                         #   use-undo-toast.ts, hidden-items.ts
│   └── utils/                    # utils.ts, formatters.ts (absorbs services/)
│
├── config/                       # layer 0 — imports nothing from src/
│   ├── routes.ts                 # ONLY route builders file (was lib/routes.ts)
│   ├── env.ts                    # typed import.meta.env (replaces vite-env.d.ts)
│   └── constants/
│       └── jobStatus.ts          # was src/constants/jobStatus.ts
│
└── types/                        # shared types used across 2+ features
    └── index.ts                  # shrinks as domain types migrate to features/<x>/types/
```

### Deleted entirely

- `src/services/` — its one file moves to `src/lib/utils/formatters.ts`
- `src/constants/routes.ts` — deleted; `src/config/routes.ts` is the only routes file
- `src/constants/` folder — jobStatus moves to `src/config/constants/`
- `src/api/` top-level folder — all 9 files move into feature `api/` subfolders

---

## 5. The six import rules (linter-enforced)

Implemented via `eslint-plugin-boundaries` in `eslint.config.js`. **These are errors in CI — not
warnings.**

```
Rule 1: config     (layer 0) → imports nothing from src/
Rule 2: lib        (layer 1) → imports nothing from features/, pages/, app/
Rule 3: components (layer 2) → imports nothing from features/, pages/, app/
Rule 4: features/<X>         → imports nothing from features/<Y>  (cross-feature ban)
Rule 5: features/<X>/internals → not importable from outside the feature
         (consumers must import from features/<X>/index.ts only)
Rule 6: pages/               → imports nothing from app/
```

Rule 4 is the primary "fix A breaks B" guard. If `jobs` needs something from `resumes`, it must
be lifted to a `shared/` component or composed at the page level. The linter makes covert coupling
impossible.

Rule 5 is the public-API contract rule. Nothing outside a feature may import from
`@/features/<domain>/components/Foo` — it must use `@/features/<domain>` (the barrel). This means
every cross-feature dependency is explicit and reviewable.

### Rollout: warn first, then error

Boundary rules ship as **warn** in the first PR so the full violation list is surfaced without
breaking CI. Violations are fixed in subsequent PRs (they are already catalogued above). The final
PR flips all rules from `warn` to `error`.

---

## 6. Per-feature public API contract

Every feature **must** export only through its `index.ts`. That file is the contract — a deliberate
API surface. Everything inside the feature folder is an implementation detail invisible to the
outside world.

```ts
// features/jobs/index.ts — example public API
export { JobsTable } from './components/JobsTable'
export { JobDetailPanel } from './components/JobDetailPanel'
export { JobFilters } from './components/JobFilters'
export { useJobs } from './hooks/useJobs'
export { useJobActions } from './hooks/useJobActions'
export { jobsUrl } from './hooks/useJobUrlState'
export type { Job, JobFilters as JobFilterParams } from './api/jobs.api'
// ← types exported from the zod schema, not from types/index.ts
```

Changing `index.ts` is a deliberate, diff-visible act — AI agents and engineers cannot accidentally
expose or consume internals.

---

## 7. The "strict AI" layer — four enforcement mechanisms

AI agents (Claude Code, Copilot, etc.) are just fast engineers. They make the same mistakes
engineers make: wrong file location, incorrect imports, invented structure. These four mechanisms
make correct behavior the path of least resistance.

**1. The linter wall**
An illegal import is a lint error — `npm run lint` fails before a human sees it. Agents self-correct
in the red-green loop because they run the gates before claiming done.

**2. Per-layer `CLAUDE.md` files**
Auto-load when an agent opens any file in that directory. Each states:

- "You are in layer N. You may import from layers 0–(N−1) only."
- "What belongs here and what does not."
- "The feature public API is `index.ts` — do not import from deeper paths."

**3. Feature scaffolding command**
`npm run scaffold feature <name>` (plop/hygen) generates the canonical skeleton:
`api/`, `components/`, `hooks/`, `types/`, `constants/`, `queryKeys.ts`, `index.ts`
with a zod schema stub and empty barrel. An agent runs the scaffolder instead of
inventing structure from scratch. The generated skeleton is always correct.

**4. Zod at the API boundary** (Phase 0.5 — already planned)
Schemas co-located in `features/<x>/api/<x>.api.ts`. Types derived via `z.infer`. Runtime
validation + TS types cannot drift from each other. The API layer is correct by construction.

---

## 8. Migration map — what moves, what is deleted

| From                                         | To                                                              | Action                              |
| -------------------------------------------- | --------------------------------------------------------------- | ----------------------------------- |
| `src/api/jobs.ts`                            | `src/features/jobs/api/jobs.api.ts`                             | Move                                |
| `src/api/companies.ts`                       | `src/features/companies/api/companies.api.ts`                   | Move                                |
| `src/api/adminCompanies.ts`                  | `src/features/companies/api/adminCompanies.api.ts`              | Move                                |
| `src/api/dashboard.ts`                       | `src/features/dashboard/api/dashboard.api.ts`                   | Move                                |
| `src/api/insights.ts`                        | `src/features/insights/api/insights.api.ts`                     | Move                                |
| `src/api/adapterConfigs.ts`                  | `src/features/settings/api/adapterConfigs.api.ts`               | Move                                |
| `src/api/adminUsers.ts`                      | `src/features/auth/api/adminUsers.api.ts`                       | Move                                |
| `src/api/adminAudit.ts`                      | `src/features/auth/api/adminAudit.api.ts`                       | Move                                |
| `src/api/me.ts`                              | `src/features/auth/api/me.api.ts`                               | Move                                |
| `src/services/jobs.service.ts`               | `src/lib/utils/formatters.ts`                                   | Move + delete `services/`           |
| `src/constants/routes.ts`                    | **Deleted**                                                     | `src/config/routes.ts` is canonical |
| `src/lib/routes.ts`                          | `src/config/routes.ts`                                          | Move                                |
| `src/constants/jobStatus.ts`                 | `src/config/constants/jobStatus.ts`                             | Move                                |
| `src/lib/api-client.ts` + infra              | `src/lib/core/`                                                 | Move into subfolder                 |
| `src/lib/use-*.ts` + hidden-items            | `src/lib/hooks/`                                                | Move into subfolder                 |
| `src/lib/utils.ts`                           | `src/lib/utils/utils.ts`                                        | Move into subfolder                 |
| `src/types/index.ts`                         | Domain types → `features/<x>/types/`; shared → stays            | Decompose                           |
| `src/components/Sidebar.tsx`                 | Fix: receive auth state from `app/` as prop or context          | Fix upward violation                |
| `features/jobs` → `features/resumes` imports | Compose at page level or lift to `shared/`                      | Fix cross-feature violation         |
| `features/insights/` (no index)              | Add `index.ts`                                                  | Add barrel                          |
| `features/dashboard/` panels at root         | Move into `components/` subfolder                               | Normalize                           |
| `features/auth/` flat layout                 | Move into `components/`, `store/`, `api/`                       | Normalize                           |
| `App.tsx` monolith                           | Split into `app/App.tsx`, `app/providers.tsx`, `app/router.tsx` | Split                               |

### What does NOT change

- Stack: React 19, TanStack Query v5, Zustand, Tailwind, Radix UI, Vite, Vitest — untouched.
- All feature logic, component JSX, hook implementations — untouched.
- The seam-test-coverage plan, Phase 0 strict typing, Phase 0.5 zod — continue in sequence.
- File names (only locations change; `@/` alias paths update but identifiers stay the same).

---

## 9. Rollout sequence (zero big-bang risk)

Each step is an independent PR. The app works at every step.

| Step | PR name                                               | What changes                                                                            |
| ---- | ----------------------------------------------------- | --------------------------------------------------------------------------------------- |
| 1    | `chore: add eslint-plugin-boundaries (warn mode)`     | Install + configure boundary rules as warnings; expose full violation list              |
| 2    | `fix: resolve existing boundary violations`           | Fix Sidebar upward import, jobs→resumes cross import, `services/` deletion, dual routes |
| 3    | `refactor: restructure lib/ into core/ hooks/ utils/` | Pure file moves inside `lib/`, update `@/lib/` import paths                             |
| 4    | `refactor: move api/ into feature api/ subfolders`    | 9 files move; imports updated; no logic change                                          |
| 5    | `refactor: normalize feature shapes`                  | insights `index.ts`, dashboard `components/`, auth subfolder, App.tsx split             |
| 6    | `refactor: decompose types/index.ts`                  | Domain types move to `features/<x>/types/`, shared types stay                           |
| 7    | `refactor: add config/ layer, merge routes`           | `config/routes.ts`, `config/env.ts`, `config/constants/`                                |
| 8    | `feat: add feature scaffolding command`               | plop/hygen template matching canonical feature shape                                    |
| 9    | `chore: add per-layer CLAUDE.md files`                | 6 layer CLAUDE.md files (enforces AI placement rules)                                   |
| 10   | `chore: promote boundary rules to errors`             | Flip warn → error; architecture is now machine-enforced                                 |

---

## 10. Success criteria

- [ ] `npm run lint` fails on any import that crosses a layer boundary
- [ ] Every `features/<x>/` has an `index.ts` and a canonical shape (api/ components/ hooks/ queryKeys.ts)
- [ ] Zero files in `src/services/`, `src/api/` (top-level), `src/constants/`
- [ ] Exactly one route builder file: `src/config/routes.ts`
- [ ] `types/index.ts` contains only types used by 2+ features; all others live in their feature
- [ ] `npm run scaffold feature <name>` produces a correct, linter-clean skeleton
- [ ] Per-layer `CLAUDE.md` exists at `src/lib/CLAUDE.md`, `src/features/CLAUDE.md`,
      `src/components/CLAUDE.md`, `src/pages/CLAUDE.md`, `src/app/CLAUDE.md`, `src/config/CLAUDE.md`
- [ ] All existing tests pass; no feature behavior changes
- [ ] `npm run build` succeeds

---

## 11. Decision log

- **2026-05-28** — Architecture proposed after full audit of the current `src/` tree. Key decisions:
  - `api/` moves into feature `api/` subfolders (domain ownership over top-level concern)
  - `eslint-plugin-boundaries` chosen over `import/no-restricted-paths` for richer element-type rules
  - Boundary rules ship as warn first, error after violations are fixed (avoids CI breakage)
  - Feature public-API barrels kept (encapsulation > Vite tree-shaking micro-optimization for this app)
  - `config/` is a new layer 0 (below `lib/`) to house routes, env, and constants cleanly
  - Migration is 10 sequential PRs — no big-bang; app works at every step
  - Resumes/apply-queue deferred from seam loop; same deferral applies here
