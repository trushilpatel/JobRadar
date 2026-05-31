# Frontend Architecture Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development`
> (recommended) or `superpowers:executing-plans` to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `frontend/src/` to a 6-layer unidirectional architecture with
machine-enforced import boundaries, consistent feature shapes, and AI-proof conventions —
10 sequential PRs, app fully working after each one.

**Architecture:** Layers 0–5 (config → lib → components → features → pages → app) with
imports strictly downward-only, enforced by `eslint-plugin-boundaries` as CI errors.
Features are self-contained domain slices with a public `index.ts` barrel as the only legal
entry point for external consumers. No file has more than one clear responsibility.

**Tech Stack:** React 19, TypeScript strict, Vite, TanStack Query v5, Zustand, Tailwind,
Radix UI, Vitest, eslint-plugin-boundaries, plop (new — scaffolding only).

**Spec:** `docs/superpowers/specs/2026-05-28-frontend-architecture-design.md`
**Reference:** `frontend/ARCHITECTURE.md`

**Gates (run after every PR before committing):**

```bash
cd frontend
npm run lint
npx tsc --noEmit
npx vitest run
npm run build
```

---

## PR 1 — Add eslint-plugin-boundaries in warn mode

**Goal:** Surface every boundary violation as a warning without breaking CI. Produces the
full violation inventory that PRs 2–9 fix.

### Task 1.1 — Install eslint-plugin-boundaries

**Files:**

- Modify: `frontend/package.json` (dev dependency added by npm)

- [ ] **Install the package**

```bash
cd frontend
npm install --save-dev eslint-plugin-boundaries
```

Expected: package added to `devDependencies`, `package-lock.json` updated.

- [ ] **Verify install**

```bash
npx eslint --print-config src/App.tsx | grep -i boundaries || echo "not yet configured"
```

Expected: `not yet configured`

---

### Task 1.2 — Configure boundary rules (warn mode)

**Files:**

- Modify: `frontend/eslint.config.js`

- [ ] **Read the current eslint config**

```bash
cat eslint.config.js
```

- [ ] **Add boundaries plugin and element-type rules**

Replace the full contents of `frontend/eslint.config.js` with:

```js
import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'
import boundaries from 'eslint-plugin-boundaries'

export default defineConfig([
  globalIgnores(['dist']),

  // ── Core rules ─────────────────────────────────────────────────────────────
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'error',
      '@typescript-eslint/no-unsafe-member-access': 'error',
      '@typescript-eslint/no-unsafe-call': 'error',
      '@typescript-eslint/no-unsafe-return': 'error',
      '@typescript-eslint/no-unsafe-argument': 'error',
    },
  },

  // ── Type-unsafe rules off in tests ─────────────────────────────────────────
  {
    files: ['**/__tests__/**', '**/*.test.{ts,tsx}', 'src/test/**'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
    },
  },

  // ── Boundary rules (WARN now → error in PR 10) ────────────────────────────
  // Element types define the layers. The pattern is relative to the project root
  // (where eslint.config.js lives). Captures let rules reference the domain slug.
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { boundaries },
    settings: {
      'boundaries/elements': [
        // Layer 0 — imports nothing
        { type: 'config', pattern: 'src/config/**/*' },
        // Shared types — may import config
        { type: 'types', pattern: 'src/types/**/*' },
        // Layer 1 — framework infrastructure
        { type: 'lib', pattern: 'src/lib/**/*' },
        // Layer 2 — zero-domain UI primitives
        { type: 'components', pattern: 'src/components/**/*' },
        // Layer 3 — domain feature slices (capture domain slug for same-feature rule)
        {
          type: 'feature',
          pattern: 'src/features/([^/]+)/**/*',
          capture: ['domain'],
        },
        // Layer 4 — route-level shells
        { type: 'pages', pattern: 'src/pages/**/*' },
        // Layer 5 — app bootstrap and routing
        { type: 'app', pattern: 'src/app/**/*' },
      ],
      // Don't enforce boundaries inside test files — test helpers legitimately
      // cross layers to set up fixtures.
      'boundaries/ignore': ['**/__tests__/**', '**/*.test.{ts,tsx}', 'src/test/**'],
    },
    rules: {
      // ── Unidirectional flow ──────────────────────────────────────────────
      // Each layer may only import from layers strictly below it.
      // 'warn' → will become 'error' in PR 10 once all violations are fixed.
      'boundaries/element-types': [
        'warn',
        {
          default: 'disallow',
          rules: [
            // config: imports nothing from src/
            { from: ['config'], allow: [] },
            // types: may import config
            { from: ['types'], allow: ['config'] },
            // lib: may import config + types
            { from: ['lib'], allow: ['config', 'types'] },
            // components: may import config + types + lib
            { from: ['components'], allow: ['config', 'types', 'lib'] },
            // feature: may import all shared layers + OWN feature (same domain)
            // Cross-feature imports (different domain) are forbidden.
            {
              from: ['feature'],
              allow: [
                'config',
                'types',
                'lib',
                'components',
                ['feature', { domain: '${from.domain}' }],
              ],
            },
            // pages: may import everything except app
            {
              from: ['pages'],
              allow: ['config', 'types', 'lib', 'components', 'feature'],
            },
            // app: may import everything
            {
              from: ['app'],
              allow: ['config', 'types', 'lib', 'components', 'feature', 'pages'],
            },
          ],
        },
      ],

      // ── Feature encapsulation ────────────────────────────────────────────
      // External consumers must import from the feature's index.ts barrel only.
      // Deep imports like @/features/jobs/hooks/useJobs are forbidden.
      'boundaries/entry-point': [
        'warn',
        {
          default: 'disallow',
          rules: [{ target: ['feature'], allow: 'index.ts' }],
        },
      ],
    },
  },
])
```

- [ ] **Run lint and capture the full violation list**

```bash
npm run lint 2>&1 | grep "boundaries/" | sort | uniq -c | sort -rn | head -40
```

Expected: A list of boundary violations (warnings, not errors — CI still passes).
Save this output mentally — it's the work queue for PRs 2–9.

- [ ] **Confirm overall lint still exits 0 (warnings don't break CI)**

```bash
npm run lint; echo "Exit: $?"
```

Expected: `Exit: 0`

- [ ] **Run all gates to confirm nothing else broke**

```bash
npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GATES GREEN"
```

Expected: `ALL GATES GREEN`

---

### Task 1.3 — Commit PR 1

- [ ] **Stage and commit**

```bash
git checkout -b chore/arch-pr1-boundary-rules-warn
git add package.json package-lock.json eslint.config.js
git commit -m "chore(ui): add eslint-plugin-boundaries in warn mode

Installs eslint-plugin-boundaries and configures 6-layer element types
with unidirectional flow rules and feature encapsulation (index.ts only).
All rules are warnings for now — will flip to errors in PR 10 once
every violation is fixed. Existing violations are surfaced but do not
break CI."
```

- [ ] **Push and open PR** (use `git-push-to-pr` skill)

---

## PR 2 — Fix existing boundary violations

**Goal:** Resolve every violation the new rules flagged without moving files (that's PRs 3–7).
After this PR, all warnings from PR 1 are gone except those caused by file locations
(which the restructuring PRs fix).

### Task 2.1 — Fix Sidebar upward import (components → features violation)

`components/Sidebar.tsx` imports `useAuthStore` from `@/features/auth/store`. Components
cannot import features. The fix: move Sidebar to `src/app/` where it belongs (it's part
of the app shell, not a reusable UI component).

**Files:**

- Create: `src/app/Sidebar.tsx` (move from `src/components/Sidebar.tsx`)
- Modify: `src/App.tsx` (update import path)
- Delete: `src/components/Sidebar.tsx`

- [ ] **Create `src/app/Sidebar.tsx`**

```bash
mkdir -p src/app
cp src/components/Sidebar.tsx src/app/Sidebar.tsx
```

The file content stays identical — only the location changes.

- [ ] **Update `src/App.tsx` import**

In `src/App.tsx`, change:

```ts
// Before
import { Sidebar } from '@/components/Sidebar'
// After
import { Sidebar } from '@/app/Sidebar'
```

- [ ] **Delete old file**

```bash
rm src/components/Sidebar.tsx
```

- [ ] **Verify no other imports of the old path**

```bash
grep -rn "from '@/components/Sidebar'" src/ && echo "STALE IMPORTS" || echo "clean"
```

Expected: `clean`

---

### Task 2.2 — Fix jobs→resumes cross-feature violation

`features/jobs/components/JobResumeSection.tsx` and `features/jobs/components/jobs-columns.tsx`
import from `features/resumes` internals. Cross-feature imports are forbidden. The fix:
`JobResumeSection` is a composition of two features — it belongs in `pages/` (where
cross-feature composition is legal), not inside `features/jobs`.

**Files:**

- Create: `src/pages/jobs/JobResumeSection.tsx` (moved from feature internals)
- Modify: `src/pages/JobsPage.tsx`
- Modify: `src/features/jobs/components/JobDetailPanel.tsx` (receives section as a prop/slot)
- Modify: `src/features/jobs/components/jobs-columns.tsx` (remove resume import)

- [ ] **Move `JobResumeSection` to pages**

```bash
mkdir -p src/pages/jobs
cp src/features/jobs/components/JobResumeSection.tsx src/pages/jobs/JobResumeSection.tsx
```

Update the imports inside `src/pages/jobs/JobResumeSection.tsx` — all feature imports are
now legal since pages can import from any feature:

```ts
// These imports are now valid (pages layer can import from both features)
import { useResumes } from '@/features/resumes'
import { ResumePickerInline, ResumeStateBadge } from '@/features/resumes'
import { getMockAttachmentForJob } from '@/features/resumes' // or move to lib/utils
```

- [ ] **Remove `JobResumeSection` from `features/jobs`**

```bash
rm src/features/jobs/components/JobResumeSection.tsx
```

- [ ] **Fix `jobs-columns.tsx` resume import**

In `src/features/jobs/components/jobs-columns.tsx`, the `getMockAttachmentForJob` import
from resumes must be removed. Since this is mock data, the column that renders the resume
attachment should receive it as a prop instead of importing directly. Adjust the column
definition to accept the attachment data via the row's data structure rather than calling
the mock function directly inside the column renderer.

Read the current file, identify the exact import, and remove or replace with a prop.

- [ ] **Expose resumes public API for the moved component**

In `src/features/resumes/index.ts`, ensure these are exported:

```ts
export { useResumes } from './hooks/useResumes'
export { ResumePickerInline } from './components/ResumePickerInline'
export { ResumeStateBadge } from './components/ResumeStateBadge'
export { getMockAttachmentForJob } from './mockData'
```

- [ ] **Wire `JobResumeSection` in `JobsPage.tsx`**

Import and render `JobResumeSection` from `@/pages/jobs/JobResumeSection` inside
`JobDetailPanel` usage in `JobsPage.tsx`, passing it as a child or prop as the current
`JobDetailPanel` API allows.

---

### Task 2.3 — Delete `services/` — move to `lib/utils/formatters.ts`

**Files:**

- Create: `src/lib/utils/formatters.ts`
- Modify: all 3 files that import from `@/services/`
- Delete: `src/services/jobs.service.ts` and `src/services/` directory

- [ ] **Create `src/lib/utils/formatters.ts`**

```bash
mkdir -p src/lib/utils
```

Copy the full contents of `src/services/jobs.service.ts` into `src/lib/utils/formatters.ts`,
keeping all exports (`avatarColor`, `formatSalary`) and the `SalaryRange` import from `@/types`.

- [ ] **Find all services importers**

```bash
grep -rn "from '@/services/" src/ | grep -v __tests__
```

Expected: ~3 files. Update each one to import from `@/lib/utils/formatters` instead.

- [ ] **Delete services/**

```bash
rm -rf src/services/
```

- [ ] **Verify**

```bash
grep -rn "from '@/services/" src/ && echo "STALE" || echo "clean"
```

Expected: `clean`

---

### Task 2.4 — Delete `constants/routes.ts` (keep `lib/routes.ts` as canonical)

`constants/routes.ts` has raw string constants (e.g. `ROUTES.JOBS = '/jobs'`).
`lib/routes.ts` has typed builders (`routes.jobs()`) and is the canonical file with 54
importers. Delete the raw-string file and migrate the 3 importers of `constants/routes.ts`.

**Files:**

- Modify: files importing `@/constants/routes` (find with grep)
- Modify: `src/features/jobs/hooks/useJobUrlState.ts` (uses `ROUTES.JOBS`)
- Delete: `src/constants/routes.ts`

- [ ] **Find constants/routes importers**

```bash
grep -rn "from '@/constants/routes'" src/ | grep -v __tests__
```

- [ ] **Migrate each importer**

For each file found, replace the `ROUTES.X` usage with the equivalent `routes.X()` call
from `@/lib/routes`. Example:

```ts
// Before
import { ROUTES } from '@/constants/routes'
navigate(ROUTES.JOBS)

// After
import { routes } from '@/lib/routes'
navigate(routes.jobs())
```

For `useJobUrlState.ts` which uses `ROUTES.JOBS` in `jobsUrl()`:

```ts
// Before
import { ROUTES } from '@/constants/routes'
return serializeJobsParams(ROUTES.JOBS, filters)

// After
import { routes } from '@/lib/routes'
return serializeJobsParams(routes.jobs(), filters)
```

- [ ] **Delete constants/routes.ts**

```bash
rm src/constants/routes.ts
```

Check if `src/constants/` is now empty:

```bash
ls src/constants/
```

If only `jobStatus.ts` remains, leave the folder (it moves in PR 7).

- [ ] **Run all gates**

```bash
npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

Expected: `ALL GREEN`. Boundary warnings should now be significantly reduced.

---

### Task 2.5 — Commit PR 2

- [ ] **Stage and commit**

```bash
git checkout -b chore/arch-pr2-fix-violations
git add -u
git add src/app/Sidebar.tsx src/pages/jobs/JobResumeSection.tsx src/lib/utils/formatters.ts
git commit -m "chore(ui): fix existing boundary violations

- Move Sidebar to app/ (was components/ importing features/auth)
- Move JobResumeSection to pages/jobs/ (was cross-feature jobs→resumes)
- Delete services/; move formatters to lib/utils/formatters.ts
- Delete constants/routes.ts; migrate 3 importers to lib/routes typed builders
All boundary warnings from PR 1 that are violation-fixable are now clean.
Remaining warnings are caused by top-level api/ and lib/ structure (fixed in PRs 3-4)."
```

- [ ] **Push and open PR**

---

## PR 3 — Restructure `lib/` into `core/` `hooks/` `utils/`

**Goal:** Give `lib/` internal structure. Pure file moves + import-path updates. Zero logic changes.

**Files to move:**

| From                                           | To                                     |
| ---------------------------------------------- | -------------------------------------- |
| `src/lib/api-client.ts`                        | `src/lib/core/api-client.ts`           |
| `src/lib/api-error.ts`                         | `src/lib/core/api-error.ts`            |
| `src/lib/supabase.ts`                          | `src/lib/core/supabase.ts`             |
| `src/lib/queryClient.ts`                       | `src/lib/core/queryClient.ts`          |
| `src/lib/session-ref.ts`                       | `src/lib/core/session-ref.ts`          |
| `src/lib/use-debounced-value.ts`               | `src/lib/hooks/use-debounced-value.ts` |
| `src/lib/use-media-query.ts`                   | `src/lib/hooks/use-media-query.ts`     |
| `src/lib/use-undo-toast.ts`                    | `src/lib/hooks/use-undo-toast.ts`      |
| `src/lib/hidden-items.ts`                      | `src/lib/hooks/hidden-items.ts`        |
| `src/lib/utils.ts`                             | `src/lib/utils/utils.ts`               |
| (already exists) `src/lib/utils/formatters.ts` | stays                                  |

### Task 3.1 — Move files

- [ ] **Create subdirectories and move files**

```bash
cd src/lib
mkdir -p core hooks utils

# core — framework infrastructure
mv api-client.ts core/
mv api-error.ts core/
mv supabase.ts core/
mv queryClient.ts core/
mv session-ref.ts core/

# hooks — shared hooks
mv use-debounced-value.ts hooks/
mv use-media-query.ts hooks/
mv use-undo-toast.ts hooks/
mv hidden-items.ts hooks/

# utils — pure functions
mv utils.ts utils/

cd ../..
```

### Task 3.2 — Update all import paths

- [ ] **Find all importers of old lib paths**

```bash
grep -rn "from '@/lib/api-client'\|from '@/lib/api-error'\|from '@/lib/supabase'\|from '@/lib/queryClient'\|from '@/lib/session-ref'\|from '@/lib/use-\|from '@/lib/hidden-items'\|from '@/lib/utils'" src/ | grep -v __tests__ | grep -v node_modules
```

- [ ] **Update each import path**

For each file found, apply these replacements (use find+sed or editor):

```bash
# In bulk — run from src/ directory
find . -name "*.ts" -o -name "*.tsx" | xargs grep -l "@/lib/" | while read f; do
  sed -i '' \
    -e "s|from '@/lib/api-client'|from '@/lib/core/api-client'|g" \
    -e "s|from '@/lib/api-error'|from '@/lib/core/api-error'|g" \
    -e "s|from '@/lib/supabase'|from '@/lib/core/supabase'|g" \
    -e "s|from '@/lib/queryClient'|from '@/lib/core/queryClient'|g" \
    -e "s|from '@/lib/session-ref'|from '@/lib/core/session-ref'|g" \
    -e "s|from '@/lib/use-debounced-value'|from '@/lib/hooks/use-debounced-value'|g" \
    -e "s|from '@/lib/use-media-query'|from '@/lib/hooks/use-media-query'|g" \
    -e "s|from '@/lib/use-undo-toast'|from '@/lib/hooks/use-undo-toast'|g" \
    -e "s|from '@/lib/hidden-items'|from '@/lib/hooks/hidden-items'|g" \
    -e "s|from '@/lib/utils'|from '@/lib/utils/utils'|g" \
    "$f"
done
```

- [ ] **Update vitest path aliases in vite.config.ts / vitest.config.ts if needed**

```bash
grep -rn "lib/" vite.config.ts vitest.config.ts 2>/dev/null | head -10
```

If any absolute path aliases point to moved files, update them.

### Task 3.3 — Verify and commit

- [ ] **Run all gates**

```bash
cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

- [ ] **Verify no old paths remain**

```bash
grep -rn "from '@/lib/api-client'\|from '@/lib/utils'" src/ | grep -v "core/\|utils/" | grep -v __tests__ && echo "STALE" || echo "clean"
```

Expected: `clean`

- [ ] **Commit**

```bash
git checkout -b chore/arch-pr3-lib-structure
git add -u
git commit -m "refactor(ui): restructure lib/ into core/ hooks/ utils/

Pure file moves — no logic changes. Imports updated across all consumers.
lib/core/   — api-client, api-error, supabase, queryClient, session-ref
lib/hooks/  — use-debounced-value, use-media-query, use-undo-toast, hidden-items
lib/utils/  — utils, formatters (was services/)"
```

- [ ] **Push and open PR**

---

## PR 4 — Move `api/` into feature `api/` subfolders

**Goal:** Each domain owns its API. The top-level `src/api/` folder is deleted.
9 files move into feature slices. Zero logic changes.

**Move map:**

| From                        | To                                                 |
| --------------------------- | -------------------------------------------------- |
| `src/api/jobs.ts`           | `src/features/jobs/api/jobs.api.ts`                |
| `src/api/companies.ts`      | `src/features/companies/api/companies.api.ts`      |
| `src/api/adminCompanies.ts` | `src/features/companies/api/adminCompanies.api.ts` |
| `src/api/dashboard.ts`      | `src/features/dashboard/api/dashboard.api.ts`      |
| `src/api/insights.ts`       | `src/features/insights/api/insights.api.ts`        |
| `src/api/adapterConfigs.ts` | `src/features/settings/api/adapterConfigs.api.ts`  |
| `src/api/adminUsers.ts`     | `src/features/auth/api/adminUsers.api.ts`          |
| `src/api/adminAudit.ts`     | `src/features/auth/api/adminAudit.api.ts`          |
| `src/api/me.ts`             | `src/features/auth/api/me.api.ts`                  |

### Task 4.1 — Move API files

- [ ] **Create api/ subdirs and move files**

```bash
cd src
mkdir -p features/jobs/api
mkdir -p features/companies/api
mkdir -p features/dashboard/api
mkdir -p features/insights/api
mkdir -p features/settings/api
mkdir -p features/auth/api

mv api/jobs.ts             features/jobs/api/jobs.api.ts
mv api/companies.ts        features/companies/api/companies.api.ts
mv api/adminCompanies.ts   features/companies/api/adminCompanies.api.ts
mv api/dashboard.ts        features/dashboard/api/dashboard.api.ts
mv api/insights.ts         features/insights/api/insights.api.ts
mv api/adapterConfigs.ts   features/settings/api/adapterConfigs.api.ts
mv api/adminUsers.ts       features/auth/api/adminUsers.api.ts
mv api/adminAudit.ts       features/auth/api/adminAudit.api.ts
mv api/me.ts               features/auth/api/me.api.ts

rmdir api/  # should be empty now
cd ..
```

### Task 4.2 — Also move test files

- [ ] **Move corresponding test files**

```bash
cd src
mv api/__tests__/jobsApi.test.ts            features/jobs/api/__tests__/jobs.api.test.ts     2>/dev/null || true
mv api/__tests__/adminCompaniesApi.test.ts  features/companies/api/__tests__/adminCompanies.api.test.ts 2>/dev/null || true
mv api/__tests__/adapterConfigsApi.test.ts  features/settings/api/__tests__/adapterConfigs.api.test.ts 2>/dev/null || true
mv api/__tests__/companies.test.ts          features/companies/api/__tests__/companies.api.test.ts 2>/dev/null || true
# Move remaining test files to the appropriate feature
ls api/__tests__/ 2>/dev/null && echo "remaining test files" || echo "api/__tests__ cleared"
rmdir api/__tests__/ 2>/dev/null || true
rmdir api/ 2>/dev/null || true
cd ..
```

### Task 4.3 — Update all import paths

- [ ] **Update imports from old @/api/ paths**

```bash
find src/ -name "*.ts" -o -name "*.tsx" | xargs grep -l "from '@/api/" | while read f; do
  sed -i '' \
    -e "s|from '@/api/jobs'|from '@/features/jobs/api/jobs.api'|g" \
    -e "s|from '@/api/companies'|from '@/features/companies/api/companies.api'|g" \
    -e "s|from '@/api/adminCompanies'|from '@/features/companies/api/adminCompanies.api'|g" \
    -e "s|from '@/api/dashboard'|from '@/features/dashboard/api/dashboard.api'|g" \
    -e "s|from '@/api/insights'|from '@/features/insights/api/insights.api'|g" \
    -e "s|from '@/api/adapterConfigs'|from '@/features/settings/api/adapterConfigs.api'|g" \
    -e "s|from '@/api/adminUsers'|from '@/features/auth/api/adminUsers.api'|g" \
    -e "s|from '@/api/adminAudit'|from '@/features/auth/api/adminAudit.api'|g" \
    -e "s|from '@/api/me'|from '@/features/auth/api/me.api'|g" \
    "$f"
done
```

- [ ] **Verify no old @/api/ imports remain**

```bash
grep -rn "from '@/api/" src/ && echo "STALE" || echo "clean"
```

Expected: `clean`

### Task 4.4 — Update feature index.ts barrels to re-export from new api/ paths

For each feature, update `index.ts` to import types and functions from the new `api/`
subdirectory. Example for jobs:

- [ ] **Update `src/features/jobs/index.ts`**

```ts
// Any existing re-exports of types from '@/api/jobs' must update to:
export type { Job, JobFilters, JobsPage, JobCounts } from './api/jobs.api'
// Functions (if re-exported):
export { fetchJobs, fetchJob, fetchJobCounts, fetchJobCompanies } from './api/jobs.api'
```

Repeat for each feature's `index.ts`. The goal is that consumers of the feature who
previously imported types from `@/api/jobs` can now get them from `@/features/jobs`.

### Task 4.5 — Verify and commit

- [ ] **Run all gates**

```bash
cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

- [ ] **Commit**

```bash
git checkout -b chore/arch-pr4-api-into-features
git add -u
git commit -m "refactor(ui): move api/ into feature api/ subfolders

Each domain now owns its API declarations. The top-level src/api/ folder
is deleted. 9 files moved with import paths updated across all consumers.
Zero logic changes — only file locations change."
```

- [ ] **Push and open PR**

---

## PR 5 — Normalize feature shapes

**Goal:** Every feature follows the canonical shape. Three features are currently irregular.

### Task 5.1 — `features/insights/` — add missing `index.ts`

Currently `features/insights/` has no barrel. Pages import components directly from
feature internals (7 deep imports in `InsightsPage.tsx`).

**Files:**

- Create: `src/features/insights/index.ts`
- Modify: `src/pages/InsightsPage.tsx`

- [ ] **Create `src/features/insights/index.ts`**

```ts
// features/insights/index.ts — public API
export { PipelineFunnelPanel } from './PipelineFunnelPanel'
export { RoleCategoryPanel } from './RoleCategoryPanel'
export { CompanyVelocityPanel } from './CompanyVelocityPanel'
export { LocationPanel } from './LocationPanel'
export { SponsorshipTrendPanel } from './SponsorshipTrendPanel'
export { TechStackPanel } from './TechStackPanel'
export { CompanyActivityPanel } from './CompanyActivityPanel'
export { InsightCard } from './InsightCard'
```

- [ ] **Update `src/pages/InsightsPage.tsx`**

Replace the 7 individual feature-internal imports with a single import from the barrel:

```ts
// Before (7 lines):
import { PipelineFunnelPanel } from '@/features/insights/PipelineFunnelPanel'
import { RoleCategoryPanel } from '@/features/insights/RoleCategoryPanel'
// ... etc

// After (1 line):
import {
  PipelineFunnelPanel,
  RoleCategoryPanel,
  CompanyVelocityPanel,
  LocationPanel,
  SponsorshipTrendPanel,
  TechStackPanel,
  CompanyActivityPanel,
} from '@/features/insights'
```

---

### Task 5.2 — `features/dashboard/` — move root-level panels into `components/`

Currently `dashboard/` has 6 panel components at the root level. Move them into
`components/` to match the canonical shape.

**Files to move:**

```
features/dashboard/HeatmapPanel.tsx       → features/dashboard/components/HeatmapPanel.tsx
features/dashboard/InterviewingPanel.tsx  → features/dashboard/components/InterviewingPanel.tsx
features/dashboard/MarketPanel.tsx        → features/dashboard/components/MarketPanel.tsx
features/dashboard/ProgressPanel.tsx      → features/dashboard/components/ProgressPanel.tsx
features/dashboard/TechTrendsPanel.tsx    → features/dashboard/components/TechTrendsPanel.tsx
features/dashboard/VelocityPanel.tsx      → features/dashboard/components/VelocityPanel.tsx
```

- [ ] **Move the 6 panel files**

```bash
mkdir -p src/features/dashboard/components
cd src/features/dashboard
mv HeatmapPanel.tsx components/
mv InterviewingPanel.tsx components/
mv MarketPanel.tsx components/
mv ProgressPanel.tsx components/
mv TechTrendsPanel.tsx components/
mv VelocityPanel.tsx components/
cd ../../..
```

- [ ] **Update `features/dashboard/index.ts`** to re-export from new paths

```ts
export { HeatmapPanel } from './components/HeatmapPanel'
export { InterviewingPanel } from './components/InterviewingPanel'
export { MarketPanel } from './components/MarketPanel'
export { ProgressPanel } from './components/ProgressPanel'
export { TechTrendsPanel } from './components/TechTrendsPanel'
export { VelocityPanel } from './components/VelocityPanel'
```

- [ ] **Update `DashboardPage.tsx`** to import via the barrel

```ts
// Before (6 deep imports):
import { ProgressPanel } from '@/features/dashboard/ProgressPanel'
// ...

// After (1 barrel import):
import {
  ProgressPanel,
  MarketPanel,
  HeatmapPanel,
  InterviewingPanel,
  VelocityPanel,
  TechTrendsPanel,
} from '@/features/dashboard'
```

---

### Task 5.3 — `features/auth/` — add `components/` and `store/` subfolders

Currently `auth/` is flat. Move files to match the canonical shape.

**Moves:**

```
features/auth/AuthCallback.tsx   → features/auth/components/AuthCallback.tsx
features/auth/LoginPage.tsx      → features/auth/components/LoginPage.tsx
features/auth/RequireAuth.tsx    → features/auth/components/RequireAuth.tsx
features/auth/RequireAdmin.tsx   → features/auth/components/RequireAdmin.tsx
features/auth/store.ts           → features/auth/store/auth.store.ts
```

- [ ] **Move files**

```bash
mkdir -p src/features/auth/components src/features/auth/store
cd src/features/auth
mv AuthCallback.tsx components/
mv LoginPage.tsx components/
mv RequireAuth.tsx components/
mv RequireAdmin.tsx components/
mv store.ts store/auth.store.ts
cd ../../..
```

- [ ] **Update `features/auth/index.ts`**

```ts
export { LoginPage } from './components/LoginPage'
export { AuthCallback } from './components/AuthCallback'
export { RequireAuth } from './components/RequireAuth'
export { RequireAdmin } from './components/RequireAdmin'
export { useAuthStore } from './store/auth.store'
```

- [ ] **Verify all external consumers use the barrel**

```bash
grep -rn "features/auth/" src/ | grep -v "features/auth/index\|features/auth/components\|features/auth/store\|features/auth/api\|__tests__" | grep -v "from '@/features/auth'" && echo "DEEP IMPORTS REMAIN" || echo "clean"
```

Expected: `clean` (all external consumers should use `@/features/auth`)

---

### Task 5.4 — Split `App.tsx` into `app/` layer files

**Files:**

- Create: `src/app/providers.tsx`
- Create: `src/app/router.tsx`
- Create: `src/app/auth-bootstrap.tsx`
- Modify: `src/app/App.tsx` (was `src/App.tsx` — move it)

- [ ] **Create `src/app/providers.tsx`**

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { NuqsAdapter } from 'nuqs/adapters/react-router/v7'
import { Toaster } from 'sonner'
import { queryClient } from '@/lib/core/queryClient'

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <NuqsAdapter>
        <Toaster position="bottom-center" theme="dark" richColors />
        {children}
      </NuqsAdapter>
    </QueryClientProvider>
  )
}
```

- [ ] **Create `src/app/auth-bootstrap.tsx`**

Extract `AuthBootstrap` from `App.tsx` verbatim:

```tsx
import { useEffect } from 'react'
import { useAuthStore } from '@/features/auth'

export function AuthBootstrap({ children }: { children: React.ReactNode }) {
  const bootstrap = useAuthStore((s) => s.bootstrap)
  useEffect(() => {
    let unsubscribe: (() => void) | undefined
    bootstrap().then((fn) => {
      unsubscribe = fn
    })
    return () => {
      unsubscribe?.()
    }
  }, [bootstrap])
  return <>{children}</>
}
```

- [ ] **Create `src/app/router.tsx`**

Extract all `<Routes>` / `<Route>` definitions from `App.tsx` into `router.tsx`.
Bring in all page imports. The `ProtectedLayout` component lives here too.

- [ ] **Simplify `src/app/App.tsx`** (or keep as `src/App.tsx` until `app/` structure completes)

```tsx
import { BrowserRouter } from 'react-router-dom'
import { Providers } from './providers'
import { AuthBootstrap } from './auth-bootstrap'
import { AppRouter } from './router'

export default function App() {
  return (
    <BrowserRouter>
      <Providers>
        <AuthBootstrap>
          <AppRouter />
        </AuthBootstrap>
      </Providers>
    </BrowserRouter>
  )
}
```

- [ ] **Update `src/main.tsx`** entry point import if needed

```bash
grep -n "App" src/main.tsx
```

If it imports from `./App`, update to `./app/App` if you moved the file.

---

### Task 5.5 — Verify and commit PR 5

- [ ] **Run all gates**

```bash
cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

- [ ] **Commit**

```bash
git checkout -b chore/arch-pr5-normalize-feature-shapes
git add -u
git add src/features/insights/index.ts src/app/providers.tsx src/app/auth-bootstrap.tsx src/app/router.tsx
git commit -m "refactor(ui): normalize feature shapes

- insights: add missing index.ts barrel; InsightsPage imports from barrel
- dashboard: move 6 root-level panels into components/ subfolder
- auth: add components/ and store/ subfolders; update barrel
- App.tsx: split into app/providers.tsx, app/auth-bootstrap.tsx, app/router.tsx
All external consumers updated to use feature barrels. Zero logic changes."
```

- [ ] **Push and open PR**

---

## PR 6 — Decompose `types/index.ts`

**Goal:** Domain-specific types migrate to `features/<x>/types/`. `types/index.ts` keeps
only types genuinely used by 2+ features.

### Task 6.1 — Audit which types are domain-local vs shared

- [ ] **Map each type to its primary consumer**

```bash
# For each exported type in types/index.ts, count how many different features use it
for TYPE in Company ScanRun ScanStats MonthlyJobPoint CompanyAnalytics Job UserJobState JobApplication AdapterConfig AdminCompany AdminCompanyListView Identity Role User ApplyQueueItem; do
  echo -n "$TYPE: "
  grep -rln "$TYPE" src/features/ | sed 's|src/features/||' | cut -d/ -f1 | sort -u | tr '\n' ',' | sed 's/,$//'
  echo
done
```

Types used by only 1 feature → move to `features/<domain>/types/`.
Types used by 2+ features → stay in `types/index.ts`.

### Task 6.2 — Move domain-local types

Based on the audit, for each type that belongs to only one feature:

- [ ] **Create `src/features/<domain>/types/index.ts`** for each domain with local types

Example for a type used only in `jobs`:

```bash
mkdir -p src/features/jobs/types
```

```ts
// src/features/jobs/types/index.ts
// Domain-local types — not imported by any other feature.
// Re-export from the feature's public API (features/jobs/index.ts) if consumers need them.
export type { SortingState } from '../components/jobs-columns' // example
```

- [ ] **Update `types/index.ts`** to remove the migrated types

For each moved type, add a re-export tombstone comment so the migration is discoverable:

```ts
// Moved to features/jobs/types/ — import from '@/features/jobs' instead
```

Then delete the type definition. Fix any imports that break.

- [ ] **Update feature index.ts barrels** to export moved types

If a moved type was previously imported by pages from `@/types`, the feature's `index.ts`
must now export it:

```ts
export type { JobDetail } from './types'
```

### Task 6.3 — Verify and commit

- [ ] **Confirm types/index.ts only has shared types**

```bash
wc -l src/types/index.ts
```

If it's still 342 lines, audit again — more types can move.

- [ ] **Run all gates**

```bash
cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

- [ ] **Commit**

```bash
git checkout -b chore/arch-pr6-decompose-types
git add -u
git commit -m "refactor(ui): decompose types/index.ts — domain types move to features

Types used by a single feature move to features/<domain>/types/.
types/index.ts now contains only types shared across 2+ features.
Feature index.ts barrels re-export domain types for consumers."
```

- [ ] **Push and open PR**

---

## PR 7 — Add `config/` layer (merge routes, type env)

**Goal:** Create layer 0 (`config/`). Routes, env, and app-wide constants live there.
This eliminates the last top-level `constants/` folder and consolidates routes.

### Task 7.1 — Create `src/config/routes.ts`

`lib/routes.ts` moves to `config/routes.ts`. This is already the canonical routes file.

**Files:**

- Create: `src/config/routes.ts` (copy of lib/routes.ts)
- Modify: all importers of `@/lib/routes`
- Delete: `src/lib/routes.ts`

- [ ] **Move file**

```bash
mkdir -p src/config
cp src/lib/routes.ts src/config/routes.ts
rm src/lib/routes.ts
```

- [ ] **Update all importers**

```bash
find src/ -name "*.ts" -o -name "*.tsx" | xargs grep -l "from '@/lib/routes'" | while read f; do
  sed -i '' "s|from '@/lib/routes'|from '@/config/routes'|g" "$f"
done
```

- [ ] **Verify**

```bash
grep -rn "from '@/lib/routes'" src/ && echo "STALE" || echo "clean"
```

---

### Task 7.2 — Create `src/config/env.ts`

Typed access to `import.meta.env` for all VITE\_ variables.

- [ ] **Create `src/config/env.ts`**

```ts
// Typed environment variable access.
// All VITE_* vars are declared here. Components import from this module,
// never from import.meta.env directly.

const env = import.meta.env as Record<string, string | undefined>

function required(key: string): string {
  const value = env[key]
  if (!value) throw new Error(`Missing required env var: ${key}`)
  return value
}

export const ENV = {
  SUPABASE_URL: required('VITE_SUPABASE_URL'),
  SUPABASE_ANON_KEY: required('VITE_SUPABASE_ANON_KEY'),
} as const
```

- [ ] **Update `lib/core/supabase.ts`** to import from `config/env.ts`

```ts
// Before (reading import.meta.env directly):
const supabaseUrl = import.meta.env['VITE_SUPABASE_URL'] as string
// After:
import { ENV } from '@/config/env'
const supabaseUrl = ENV.SUPABASE_URL
```

---

### Task 7.3 — Move `constants/jobStatus.ts` to `config/constants/`

- [ ] **Move file**

```bash
mkdir -p src/config/constants
mv src/constants/jobStatus.ts src/config/constants/jobStatus.ts
rmdir src/constants/  # remove now-empty folder
```

- [ ] **Update all importers**

```bash
find src/ -name "*.ts" -o -name "*.tsx" | xargs grep -l "from '@/constants/jobStatus'" | while read f; do
  sed -i '' "s|from '@/constants/jobStatus'|from '@/config/constants/jobStatus'|g" "$f"
done
```

- [ ] **Verify**

```bash
grep -rn "from '@/constants/" src/ && echo "STALE" || echo "clean"
ls src/constants/ 2>/dev/null && echo "FOLDER REMAINS" || echo "constants/ deleted"
```

Expected: both `clean` and `constants/ deleted`.

---

### Task 7.4 — Verify and commit PR 7

- [ ] **Run all gates**

```bash
cd frontend && npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ALL GREEN"
```

- [ ] **Commit**

```bash
git checkout -b chore/arch-pr7-config-layer
git add -u
git add src/config/
git commit -m "refactor(ui): add config/ layer — routes, env, constants

- config/routes.ts: was lib/routes.ts (all imports updated)
- config/env.ts: typed import.meta.env access (no more direct env reads)
- config/constants/jobStatus.ts: was constants/jobStatus.ts
- src/constants/ folder deleted (was last residual top-level constants)
Layer 0 is now established. All other layers import routes/env/constants from config/."
```

- [ ] **Push and open PR**

---

## PR 8 — Feature scaffolding command

**Goal:** `npm run scaffold feature <name>` generates a canonical, lint-clean skeleton.
Agents and engineers run this instead of inventing structure.

### Task 8.1 — Install plop

- [ ] **Install plop**

```bash
cd frontend
npm install --save-dev plop
```

### Task 8.2 — Create plopfile and templates

- [ ] **Create `frontend/plopfile.js`**

```js
export default function (plop) {
  plop.setGenerator('feature', {
    description: 'Scaffold a new feature slice under src/features/',
    prompts: [
      {
        type: 'input',
        name: 'name',
        message: 'Feature name (kebab-case, e.g. "notifications"):',
        validate: (v) => /^[a-z][a-z0-9-]*$/.test(v) || 'Use lowercase kebab-case',
      },
    ],
    actions: [
      // queryKeys.ts
      {
        type: 'add',
        path: 'src/features/{{name}}/queryKeys.ts',
        templateFile: 'plop-templates/feature/queryKeys.ts.hbs',
      },
      // api stub
      {
        type: 'add',
        path: 'src/features/{{name}}/api/{{name}}.api.ts',
        templateFile: 'plop-templates/feature/api.ts.hbs',
      },
      // index.ts barrel
      {
        type: 'add',
        path: 'src/features/{{name}}/index.ts',
        templateFile: 'plop-templates/feature/index.ts.hbs',
      },
      // placeholder files so git tracks empty dirs
      { type: 'add', path: 'src/features/{{name}}/components/.gitkeep', template: '' },
      { type: 'add', path: 'src/features/{{name}}/hooks/.gitkeep', template: '' },
      { type: 'add', path: 'src/features/{{name}}/types/.gitkeep', template: '' },
    ],
  })
}
```

- [ ] **Create `frontend/plop-templates/feature/queryKeys.ts.hbs`**

```ts
export const {{camelCase name}}QueryKeys = {
  all: () => ['{{name}}'] as const,
  list: () => ['{{name}}', 'list'] as const,
  detail: (id: string) => ['{{name}}', 'detail', id] as const,
}
```

- [ ] **Create `frontend/plop-templates/feature/api.ts.hbs`**

```ts
import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'

// ── Schemas (source of truth — TS types are derived via z.infer) ─────────────

export const {{pascalCase name}}Schema = z.object({
  id: z.string(),
  // TODO: add fields matching the backend response shape
})

export type {{pascalCase name}} = z.infer<typeof {{pascalCase name}}Schema>

// ── API functions ─────────────────────────────────────────────────────────────

export async function fetch{{pascalCase name}}(id: string): Promise<{{pascalCase name}}> {
  const raw = await apiClient.get<unknown>(`/{{name}}/${id}`)
  return {{pascalCase name}}Schema.parse(raw)
}
```

- [ ] **Create `frontend/plop-templates/feature/index.ts.hbs`**

```ts
// {{pascalCase name}} feature — public API
// Only items exported here are accessible to consumers outside this feature.
// Everything else is an implementation detail.

// export { {{pascalCase name}}Component } from './components/{{pascalCase name}}Component'
// export { use{{pascalCase name}} } from './hooks/use{{pascalCase name}}'
export type { {{pascalCase name}} } from './api/{{name}}.api'
```

- [ ] **Add scaffold script to `package.json`**

In `frontend/package.json`, add to `scripts`:

```json
"scaffold": "plop"
```

### Task 8.3 — Test the scaffolder

- [ ] **Run the scaffolder for a test feature**

```bash
cd frontend
npm run scaffold feature -- --name testfeature
```

Expected: files created at `src/features/testfeature/`.

- [ ] **Verify the generated files pass lint and tsc**

```bash
npx eslint src/features/testfeature/ && npx tsc --noEmit && echo "SCAFFOLD CLEAN"
```

Expected: `SCAFFOLD CLEAN`

- [ ] **Delete the test feature**

```bash
rm -rf src/features/testfeature/
```

### Task 8.4 — Commit PR 8

```bash
git checkout -b chore/arch-pr8-scaffolding
git add package.json package-lock.json plopfile.js plop-templates/
git commit -m "feat(ui): add feature scaffolding command (npm run scaffold feature <name>)

Generates canonical feature skeleton: api/, components/, hooks/, types/,
queryKeys.ts, index.ts with zod schema stub. Generated output passes lint
and tsc. Agents and engineers run this instead of inventing structure."
```

---

## PR 9 — Per-layer `CLAUDE.md` files

**Goal:** Auto-loading instructions for AI agents in each layer directory. Agents read
these when they open any file in the layer and know exactly what belongs and what doesn't.

### Task 9.1 — Write 6 per-layer CLAUDE.md files

- [ ] **Create `src/config/CLAUDE.md`**

```markdown
# config/ — Layer 0

This is the bottom of the import stack. config/ imports NOTHING from src/.

## What belongs here

- Route builder functions (routes.ts)
- Typed environment variables (env.ts)
- App-wide constants (constants/)

## What does NOT belong here

- Any import from lib/, components/, features/, pages/, or app/
- Business logic, hooks, or components

## Rules

- NEVER add `import { x } from '@/lib/...'` or any higher layer
- All VITE\_\* env vars are accessed via env.ts, not import.meta.env directly
- New route builders go in routes.ts, new constants in constants/<name>.ts
```

- [ ] **Create `src/lib/CLAUDE.md`**

```markdown
# lib/ — Layer 1: Framework Infrastructure

## What belongs here

- lib/core/ — HTTP client, Supabase client, QueryClient, session ref
- lib/hooks/ — shared hooks used by 2+ features (debounce, media query, undo)
- lib/utils/ — pure utility functions (cn, formatters, etc.)

## What does NOT belong here

- Anything domain-specific (that belongs in features/<domain>/)
- React components or JSX
- Imports from features/, pages/, or app/

## Rules

- May import from: config/, types/
- May NOT import from: features/, components/, pages/, app/
- If a hook is only used by one feature, it belongs in features/<domain>/hooks/
- If a utility is only used by one feature, it belongs in features/<domain>/utils/
```

- [ ] **Create `src/components/CLAUDE.md`**

```markdown
# components/ — Layer 2: Shared UI

## What belongs here

- components/ui/ — Radix-based primitives, zero domain knowledge
- components/shared/ — lightly domain-aware components used by 2+ features

## What does NOT belong here

- Feature-specific components (those go in features/<domain>/components/)
- Business logic, data fetching, or Zustand stores
- Imports from features/, pages/, or app/

## Rules

- May import from: config/, types/, lib/
- May NOT import from: features/, pages/, app/
- If a component uses domain data (e.g. a job status badge), it is domain-specific
  and belongs in features/<domain>/components/, not here
- No prop drilling of domain types — use typed props from types/ only
```

- [ ] **Create `src/features/CLAUDE.md`**

```markdown
# features/ — Layer 3: Domain Slices

Each subdirectory is a self-contained domain slice.

## Canonical shape of every feature
```

features/<domain>/
api/ fetch functions + zod schemas + z.infer types
components/ UI components for this domain
hooks/ hooks for this domain  
 store/ Zustand slice (if needed)
types/ domain-local types (not shared app-wide)
queryKeys.ts query key factory
index.ts PUBLIC API — the ONLY legal entry point for consumers

```

## Creating a new feature
Run: `npm run scaffold feature <name>`
Do NOT hand-write the structure.

## The iron rules
1. NEVER import from another feature: `import x from '@/features/other'` is FORBIDDEN
2. NEVER import from pages/ or app/
3. External code MUST import from index.ts only — never from deep paths
4. May import from: config/, types/, lib/, components/

## index.ts is the contract
If a component, hook, or type is not exported from index.ts, it does not exist
to the outside world. Adding to index.ts is a deliberate, reviewable act.
```

- [ ] **Create `src/pages/CLAUDE.md`**

```markdown
# pages/ — Layer 4: Route Shells

Pages compose features. They do not own business logic or data fetching.

## What belongs here

- Route-level components that compose 2+ features
- Cross-feature composition (e.g. a job detail panel that also shows resume data)

## What does NOT belong here

- Data fetching (that belongs in feature hooks)
- Business logic (that belongs in feature hooks/stores)
- Reusable components (those belong in features/<domain>/components/ or components/shared/)
- Imports from app/

## Rules

- May import from: config/, types/, lib/, components/, features/
- May NOT import from: app/
- Import features via their barrel (index.ts) only — NEVER from deep paths
- If a page has more than 150 lines, it is probably doing too much
```

- [ ] **Create `src/app/CLAUDE.md`**

```markdown
# app/ — Layer 5: Application Shell

The top of the stack. Composes everything.

## What belongs here

- App.tsx — entry point, mounts providers and router
- providers.tsx — QueryClientProvider, NuqsAdapter, Toaster
- router.tsx — all <Route> definitions
- auth-bootstrap.tsx — session bootstrap logic
- Sidebar.tsx — app shell navigation (knows about auth, must live here)

## Rules

- May import from any layer (config → lib → components → features → pages)
- Nothing in lower layers should import from app/
- Keep files thin — app/ is wiring, not logic
- New routes go in router.tsx, new providers in providers.tsx
```

### Task 9.2 — Commit PR 9

```bash
git checkout -b chore/arch-pr9-layer-claude-mds
git add src/config/CLAUDE.md src/lib/CLAUDE.md src/components/CLAUDE.md
git add src/features/CLAUDE.md src/pages/CLAUDE.md src/app/CLAUDE.md
git commit -m "docs(ui): add per-layer CLAUDE.md files for AI agent guidance

Each layer directory now has a CLAUDE.md that auto-loads when an agent
edits files there. States: what layer this is, what may be imported,
what must not be imported, and where to place new files. Machine-readable
complement to the linter rules."
```

---

## PR 10 — Promote boundary rules to errors (final gate)

**Goal:** Flip all `'warn'` to `'error'` in boundary rules. After this PR, an incorrect
import fails CI instantly. The architecture is machine-enforced.

### Task 10.1 — Verify zero warnings remain

- [ ] **Run lint and count boundary warnings**

```bash
cd frontend
npm run lint 2>&1 | grep "boundaries/" | wc -l
```

Expected: `0`. If any remain, fix them before proceeding — PRs 2–9 should have
eliminated all violations.

### Task 10.2 — Flip warn → error in `eslint.config.js`

- [ ] **Update `eslint.config.js`**

In the boundaries config block, change both rule severities:

```js
// Before:
'boundaries/element-types': ['warn', { ... }],
'boundaries/entry-point': ['warn', { ... }],

// After:
'boundaries/element-types': ['error', { ... }],
'boundaries/entry-point': ['error', { ... }],
```

### Task 10.3 — Run all gates one final time

- [ ] **Full gate run**

```bash
npm run lint && npx tsc --noEmit && npx vitest run && npm run build && echo "ARCHITECTURE ENFORCED"
```

Expected: `ARCHITECTURE ENFORCED` — zero errors, zero warnings from boundaries.

### Task 10.4 — Update ARCHITECTURE.md status line

- [ ] **Update `frontend/ARCHITECTURE.md`**

Change the status line from:

```
**Status:** Migration in progress — see `Designs/seam-test-coverage-plan.md`
```

To:

```
**Status:** Complete — boundaries enforced as CI errors as of <date>
```

### Task 10.5 — Update seam-test-coverage-plan.md

- [ ] **Mark Phase 0.7 as Done** in `Designs/seam-test-coverage-plan.md`

Add a row for Phase 0.7 in the status table (or update the note) to record that the
architecture migration is complete and boundary rules are errors.

### Task 10.6 — Commit PR 10

```bash
git checkout -b chore/arch-pr10-boundaries-as-errors
git add eslint.config.js frontend/ARCHITECTURE.md Designs/seam-test-coverage-plan.md
git commit -m "chore(ui): promote boundary rules from warn to error

All 10 architectural migration PRs complete. Every layer boundary is now
a CI error — illegal imports fail lint before a human reviews the code.
Feature encapsulation (index.ts-only imports) is also enforced as an error.
The architecture cannot silently drift regardless of who — or what — writes the code."
```

---

## Self-Review Checklist

### Spec coverage

| Spec requirement                             | Task                                          |
| -------------------------------------------- | --------------------------------------------- |
| 6-layer unidirectional model                 | Tasks 1.2, 10.2                               |
| `eslint-plugin-boundaries` CI enforcement    | Tasks 1.1, 1.2, 10.2                          |
| Feature public `index.ts` contract           | Tasks 5.1, 5.2 (and existing index.ts files)  |
| Warn → error rollout                         | Tasks 1.2, 10.2                               |
| Delete `services/`                           | Task 2.3                                      |
| Delete `constants/routes.ts`                 | Task 2.4                                      |
| Delete top-level `api/`                      | Task 4.1                                      |
| Restructure `lib/` into core/hooks/utils     | Task 3.1                                      |
| Normalize feature shapes                     | Tasks 5.1–5.4                                 |
| Decompose `types/index.ts`                   | Tasks 6.1–6.2                                 |
| Add `config/` layer (routes, env, constants) | Tasks 7.1–7.3                                 |
| Feature scaffolding command                  | Tasks 8.1–8.3                                 |
| Per-layer CLAUDE.md files                    | Task 9.1                                      |
| Fix Sidebar upward import                    | Task 2.1                                      |
| Fix jobs→resumes cross-feature import        | Task 2.2                                      |
| Split App.tsx into app/ layer                | Task 5.4                                      |
| All existing tests pass after each PR        | Tasks 1.2, 2.5, 3.3, 4.5, 5.5, 6.3, 7.4, 10.3 |

All spec requirements covered. ✓

### Type consistency

- `routes` (from `@/config/routes`) is consistently referenced as `routes.jobs()` etc.
- `ENV` (from `@/config/env`) is the consistent export name.
- Feature API files consistently named `<domain>.api.ts`.
- Barrel files consistently named `index.ts`.

### No placeholders

Task 2.2 (JobResumeSection) contains intentional flexibility in the wiring — the
exact prop API depends on how `JobDetailPanel` currently accepts children. The instruction
to "read the current file and adjust" is necessary because the precise prop-threading
depends on the current API, which may change between PRs. This is noted, not a bug.
