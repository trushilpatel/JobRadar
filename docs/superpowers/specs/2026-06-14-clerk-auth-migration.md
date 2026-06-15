# 2026-06-14 — Clerk Auth Migration

Full replacement of Supabase auth with Clerk across backend and frontend.

## What was built

**Backend**

- `backend/migrations/081_clerk_auth_schema.sql` — truncates all user data, drops `roles`/`user_roles`, alters `users.id` and all FK columns from UUID to TEXT (Clerk IDs are strings)
- `backend/common/config/config.go` — `AuthConfig` now holds `ClerkJWKSURL`/`ClerkIssuerURL`; `MultiUserConfig` holds `ClerkSecretKey`; old Supabase auth fields removed
- `backend/apps/api/middleware/auth.go` — rewrote JWT verification using `github.com/MicahParks/keyfunc/v3`; returns `VerifiedClaims{Subject,Email,Role}`; supports RS256 and ES256; clock skew hard-coded 60s; dev-bypass grants `Role:"admin"`
- `backend/apps/api/middleware/identity.go` — `RequireIdentity` drops `roleService` parameter entirely; role set from `VerifiedClaims.Role` (JWT claim, no DB lookup); empty role → `[]string{}` (not nil)
- `backend/platform/clerkadmin/admin.go` — new `Client` interface + `RESTClient` for Clerk admin API; `DeleteUser` calls `DELETE /v1/users/{id}` with Bearer token; 404 → `ErrUserNotFound`
- `backend/platform/supabaseadmin/` — deleted
- `backend/service/role/` — deleted
- `backend/apps/api/wire_multiuser.go` / `main.go` — wired to `clerkadmin.Client`; role service removed

**Frontend**

- `frontend/src/config/env.ts` — exports `CLERK_PUBLISHABLE_KEY` (removed Supabase keys)
- `frontend/src/main.tsx` — wrapped with `<ClerkProvider>`
- `frontend/src/lib/core/api-client.ts` — async token via `window.Clerk.session.getToken()`; 401 → `Clerk.signOut()` + redirect (no retry loop)
- `frontend/src/features/auth/hooks/useIdentity.ts` — new hook wrapping `useAuth`/`useUser` + TanStack Query `fetchMe`; `isAdmin` from `publicMetadata.role`
- `frontend/src/features/auth/components/SignInPage.tsx` — Clerk `<SignIn>` component; redirects to `/` when already signed in
- `frontend/src/features/auth/components/RequireAuth.tsx` — uses `useAuth()` from Clerk
- `frontend/src/features/auth/components/RequireAdmin.tsx` — uses `useUser().publicMetadata.role`
- Deleted: `auth.store.ts`, `supabase.ts`, `session-ref.ts`, `auth-bootstrap.tsx`, `LoginPage.tsx`, `AuthCallback.tsx`
- Router: `/login` → `<SignInPage>`; `/auth/callback` route removed

## Coverage achieved

- `apps/api/middleware`: 97.8%
- `platform/clerkadmin`: 95.5%
- `common/config`: 97.6%
- Frontend: 1108 tests passing across 94 test files

## Key invariants

- Admin role is set via Clerk dashboard (`publicMetadata.role = "admin"`), never via DB
- JWT template must emit `{"email":"{{user.primary_email_address}}","role":"{{user.public_metadata.role}}"}` custom claims
- `users.id` is now TEXT — Clerk subject IDs (e.g. `user_xxx`) not UUIDs
