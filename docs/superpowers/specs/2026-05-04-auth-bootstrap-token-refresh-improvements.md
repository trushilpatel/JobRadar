# Auth Bootstrap & Token Refresh Improvements

**Date:** 2026-05-04

## Problem

Three latency sources in the auth/API layer:

1. **Bootstrap blocked rendering** — `loading: true` persisted until both `getSession` and `fetchMe` completed sequentially, delaying all TanStack Query starts.
2. **Reactive token refresh** — expired tokens caused a full 401 round-trip before `refreshSession()` was called, doubling request latency on refresh cycles.
3. **Redundant `fetchMe` on token refresh** — `onAuthStateChange` called `loadIdentity()` for every event including `TOKEN_REFRESHED`, adding an unnecessary `/api/me` call after every refresh.

## Changes

### `ui/console/src/features/auth/store.ts`

- `bootstrap()` now sets `loading: false` immediately after `getSession` (localStorage read), then loads identity in the background via an un-awaited `.then()`. This unblocks `RequireAuth` and page queries before `/api/me` completes.
- Background identity `.then()` guards with `get().session` — bails out if the user signed out during the fetch, preventing a stale set from overwriting a SIGNED_OUT state.
- `onAuthStateChange` handler returns early on `TOKEN_REFRESHED`, updating only `session` without calling `loadIdentity()`.

### `ui/console/src/lib/api-client.ts`

- `authHeader()` checks `session.expires_at` before attaching the token. If the token expires within 60 seconds, `refreshOnce()` is called proactively and a fresh session is fetched. The `expiresAt > 0` guard ensures sessions without an expiry field fall back to the reactive 401 path unchanged.

### `ui/console/src/features/auth/__tests__/store.test.ts` (new)

Covers bootstrap unblocking, session-only update on `TOKEN_REFRESHED`, identity fetch on `SIGNED_IN`, and identity clear on `SIGNED_OUT`.

### `ui/console/src/features/auth/__tests__/api-client.test.ts`

Two new tests: proactive refresh when token expires within 60s (single fetch, no 401), and no proactive refresh when token has ample time.
