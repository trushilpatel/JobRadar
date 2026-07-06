import { test, expect } from '@playwright/test'

// Cross-cutting auth/admin route-gating checks. Read-only — no data mutation.
//
// Group 1 runs under the "chromium-user" project's default storageState
// (already authenticated as TEST_ROLE_USER, a non-admin Clerk account) and
// verifies the client-side admin route guard (RequireAdmin, see
// src/features/auth/components/RequireAdmin.tsx) actually blocks navigation
// to an admin-only route.
//
// Group 2 overrides storageState to a fully empty session (no cookies, no
// origins) to simulate a signed-out visitor and verifies the auth guard
// (RequireAuth, see src/features/auth/components/RequireAuth.tsx) redirects
// to /login.

test.describe('admin route gating (authenticated non-admin)', () => {
  test('non-admin user hitting /admin/companies is redirected away and never sees Companies content', async ({
    page,
  }) => {
    await page.goto('/admin/companies')

    // RequireAdmin redirects non-admins to /dashboard (a route with no
    // matching <Route> definition in app/router.tsx's nested ProtectedLayout
    // <Routes> — there is no catch-all "*" route either, so it renders as a
    // blank main content area under the sidebar shell). The important,
    // stable assertion is the negative one: the app must not stay on
    // /admin/companies and must not render the admin Companies page content.
    await expect(page).toHaveURL(/\/dashboard$/, { timeout: 10_000 })
    await expect(page).not.toHaveURL(/\/admin\/companies/)

    // CompaniesHeader renders an <h1>Companies</h1> — assert it is absent,
    // i.e. the admin-only page content never mounted for this non-admin user.
    await expect(page.getByRole('heading', { name: 'Companies', exact: true })).toHaveCount(0)
  })
})

test.describe('unauthenticated access', () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test('signed-out visitor hitting /jobs is redirected to /login', async ({ page }) => {
    await page.goto('/jobs')

    // RequireAuth redirects unauthenticated visitors to /login before the
    // protected layout (and therefore the Jobs feed) ever mounts.
    await expect(page).toHaveURL(/\/login$/, { timeout: 10_000 })

    // Confirm the actual sign-in form rendered, not just the URL — the
    // Clerk sign-in widget's identifier field is the concrete evidence the
    // login page (not some other blank/dead route) mounted.
    await expect(page.locator('input[name="identifier"]')).toBeVisible()
  })
})
