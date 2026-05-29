# config/ — Layer 0

The bottom of the import stack. `config/` imports NOTHING from `src/`.

## What belongs here

- `routes.ts` — all typed URL builder functions (`routes.jobs()`, etc.)
- `env.ts` — typed `VITE_*` environment variable access
- `constants/` — app-wide constants (`jobStatus.ts`, etc.)

## What does NOT belong here

- Any import from `lib/`, `components/`, `features/`, `pages/`, or `app/`
- Business logic, hooks, or components of any kind

## Rules

- NEVER add `import { x } from '@/lib/...'` or any higher layer
- All `VITE_*` env vars are accessed via `env.ts` — never `import.meta.env` directly elsewhere
- New route builders go in `routes.ts`; new constants in `constants/<name>.ts`
