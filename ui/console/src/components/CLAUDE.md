# components/ — Layer 2: Shared UI

## What belongs here

- `ui/` — Radix-based primitives, Tailwind-composed, zero domain knowledge
- `shared/` — lightly domain-aware components used by 2+ features (CompanyAvatar, ErrorBoundary)

## What does NOT belong here

- Feature-specific components (those go in `features/<domain>/components/`)
- Business logic, data fetching, or Zustand stores
- Imports from `features/`, `pages/`, or `app/`

## Rules

- May import from: `config/`, `types/`, `lib/`
- May NOT import from: `features/`, `pages/`, `app/`
- If a component uses domain data (e.g. job status), it is domain-specific → `features/<domain>/components/`
- No prop drilling of domain types — use typed props from `types/` only
