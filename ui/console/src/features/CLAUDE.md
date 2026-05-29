# features/ — Layer 3: Domain Slices

Each subdirectory is a self-contained domain slice.

## Canonical shape of every feature

```
features/<domain>/
  api/            fetch functions + zod schemas + z.infer types
  components/     UI components for this domain
  hooks/          hooks for this domain
  types/          domain-local types (not shared app-wide)
  queryKeys.ts    query key factory
  index.ts        PUBLIC API — the ONLY legal import path for consumers
```

## Creating a new feature

```bash
npm run scaffold feature <name>
```

Do NOT hand-write the structure — use the scaffolder.

## The iron rules

1. NEVER import from another feature: `import x from '@/features/other'` is FORBIDDEN
2. NEVER import from `pages/` or `app/`
3. External code MUST import from `index.ts` only — never from deep paths like `@/features/jobs/hooks/useJobs`
4. Within a feature, use relative imports (`../hooks/X`, `./api/X`) — never `@/features/<own-domain>/...`
5. May import from: `config/`, `types/`, `lib/`, `components/`

## index.ts is the contract

If something is not exported from `index.ts`, it does not exist to the outside world.
Adding to `index.ts` is a deliberate, reviewable act.

## API files

Every fetch function lives in `api/<domain>.api.ts` with a co-located zod schema.
The TS type is derived via `z.infer` — schema and type can never drift.
