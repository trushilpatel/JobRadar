# lib/ — Layer 1: Framework Infrastructure

## What belongs here

- `core/` — HTTP client, QueryClient, API error class, resilient response parser (Clerk auth is wired via `ClerkProvider` in `main.tsx`, not a client here)
- `hooks/` — shared hooks used by 2+ features (debounce, media query, undo, hidden-items)
- `utils/` — pure utility functions (cn, formatters, etc.)
- `api/` — shared API helpers used by 2+ features (e.g. pipeline.ts)
- `commands/` — command registry — keyboard shortcuts, dispatch, platform detection, useCommandKeyboard hook

## What does NOT belong here

- Anything domain-specific (belongs in `features/<domain>/`)
- React components or JSX
- Imports from `features/`, `components/`, `pages/`, or `app/`

## Rules

- May import from: `config/`, `types/`
- May NOT import from: `features/`, `components/`, `pages/`, `app/`
- If a hook is only used by one feature → it belongs in `features/<domain>/hooks/`
- If a utility is only used by one feature → it belongs in `features/<domain>/`
