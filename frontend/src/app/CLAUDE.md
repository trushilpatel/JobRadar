# app/ — Layer 5: Application Shell

The top of the stack. Composes everything.

## What belongs here

- `App.tsx` — entry point, mounts providers and router
- `providers.tsx` — QueryClientProvider, NuqsAdapter, Toaster
- `router.tsx` — all `<Route>` definitions
- `auth-bootstrap.tsx` — session bootstrap logic
- `Sidebar.tsx` — app shell navigation (imports auth store; lives here because app/ can import features)

## Rules

- May import from any layer (`config` → `lib` → `components` → `features` → `pages`)
- Nothing in lower layers should import from `app/`
- Keep files thin — `app/` is wiring, not logic
- New routes go in `router.tsx`; new providers go in `providers.tsx`
