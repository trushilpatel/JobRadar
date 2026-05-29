# pages/ — Layer 4: Route Shells

Pages compose features. They do not own business logic or data fetching.

## What belongs here

- Route-level components that compose 2+ features
- Cross-feature composition (e.g. a page that shows both job data and company data)
- `jobs/` subfolder — page-level components that compose 2 features (e.g. JobResumeSection)

## What does NOT belong here

- Data fetching (belongs in feature hooks)
- Business logic (belongs in feature hooks/stores)
- Reusable components (belong in `features/<domain>/components/` or `components/shared/`)
- Imports from `app/`

## Rules

- May import from: `config/`, `types/`, `lib/`, `components/`, `features/`
- May NOT import from: `app/`
- Import features via their barrel (`@/features/X`) ONLY — never from deep paths
- If a page exceeds ~150 lines, it is probably doing too much — extract to feature components
