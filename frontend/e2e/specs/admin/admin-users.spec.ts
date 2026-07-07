import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page } from '@playwright/test'
import { AdminUsersPage } from '../../pages/admin/AdminUsersPage'
import {
  createDisposableClerkUser,
  deleteDisposableClerkUser,
  type DisposableClerkUser,
} from '../../fixtures/disposable-clerk-user'

const dirname = path.dirname(fileURLToPath(import.meta.url))
const adminStorageStatePath = path.join(dirname, '../../.auth/admin.json')

// Signs a disposable user in through the real Clerk UI (verified sequence)
// in a fresh, throwaway browser context — this is what provisions their
// backend `users` row via the app's auto-provision-on-first-request
// behavior. Never reuses the admin project's own storageState.
async function provisionDisposableUser(
  browser: import('@playwright/test').Browser,
  disposable: DisposableClerkUser
) {
  // IMPORTANT: inside the Playwright test runner, `browser.newContext()`
  // with no options silently inherits the project's `use.storageState`
  // (this project's chromium-admin project sets it to admin.json) as a
  // default. Passing `storageState: undefined` explicitly overrides that
  // default so this context truly starts logged out — without it, this
  // "fresh" context is actually already authenticated as the admin test
  // role account, /login redirects straight to /jobs, and the identifier
  // fill() below hangs forever waiting for a form that will never render.
  const context = await browser.newContext({ storageState: undefined })
  const page = await context.newPage()
  try {
    await page.goto('/login')
    await page.locator('input[name="identifier"]').fill(disposable.email)
    await page.locator('input[name="password"]').fill(disposable.password)
    await page.getByRole('button', { name: 'Continue', exact: true }).click()
    await page.waitForTimeout(1000)
    await page.keyboard.type(disposable.otp, { delay: 50 })
    await expect(page).toHaveURL(/\/jobs/, { timeout: 15_000 })
  } finally {
    await context.close()
  }
}

// Retries the suspend/reinstate click if it hits a transient 500 from the
// PATCH /admin/users/:id/status endpoint. This local dev stack's Go API runs
// under `air` hot-reload and is shared by many concurrent agents editing
// backend/ files in this environment — a request that lands mid-rebuild-
// restart genuinely 500s once in a while, independent of this test's logic.
// Retrying the SAME click (not weakening the assertion) is safe here because
// a failed attempt leaves the row's status unchanged, so re-clicking repeats
// the identical transition rather than double-toggling.
async function toggleSuspendWithRetry(
  page: Page,
  adminUsers: AdminUsersPage,
  email: string,
  successText: string
) {
  const maxAttempts = 3
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    await adminUsers.toggleSuspend(email)
    const success = page.getByText(successText)
    const failure = page.getByText('Failed to update status')
    const outcome = await Promise.race([
      success.waitFor({ state: 'visible', timeout: 8_000 }).then(() => 'success' as const),
      failure.waitFor({ state: 'visible', timeout: 8_000 }).then(() => 'failure' as const),
    ]).catch(() => 'timeout' as const)
    if (outcome === 'success') return
    if (attempt === maxAttempts) {
      throw new Error(
        `toggleSuspend for ${email} did not succeed after ${maxAttempts} attempts (last outcome: ${outcome})`
      )
    }
    // Let the failure toast clear before retrying so the next success toast
    // (identical text on suspend vs reinstate is impossible here since the
    // two actions have distinct successText) isn't shadowed by a stale one.
    await page.waitForTimeout(500)
  }
}

test.describe('Admin Users page', () => {
  let disposable: DisposableClerkUser

  test.afterAll(async () => {
    if (disposable) {
      // Safety-net cleanup — no-op (404) if the UI delete flow already removed the user.
      await deleteDisposableClerkUser(disposable.clerkUserId)
    }
  })

  test('provision, suspend, reinstate, then delete a disposable user', async ({ browser }) => {
    disposable = await createDisposableClerkUser('user')

    // Precondition: the disposable user has no backend `users` row yet. Sign
    // them in once (fresh context, done BEFORE any admin-authenticated
    // context/page exists in this browser instance) so their first
    // authenticated API call (loading /jobs) auto-provisions the row.
    //
    // Note: this deliberately avoids requesting the `page` fixture on the
    // test signature — Playwright eagerly materializes any fixture named in
    // the destructured params (including the project's pre-authenticated
    // admin storageState) before the test body runs. Clerk's local dev
    // instance ties a signed-in session to the app's origin/port rather than
    // strictly to browser-context cookies, so an already-open authenticated
    // tab in the same browser process can hijack a concurrent fresh sign-in
    // attempt in another context. Doing the disposable sign-in first, with
    // no other authenticated context open yet, avoids that collision.
    await provisionDisposableUser(browser, disposable)

    // Only now open the admin-authenticated context (loaded explicitly from
    // the same storageState file the chromium-admin project itself uses).
    const adminContext = await browser.newContext({ storageState: adminStorageStatePath })
    try {
      const page = await adminContext.newPage()
      const adminUsers = new AdminUsersPage(page)
      await adminUsers.goto()

      // Filter down to just this disposable user by email so we never risk
      // touching the shared TEST_ROLE_USER_EMAIL / TEST_ROLE_ADMIN_EMAIL rows.
      await adminUsers.searchByEmail(disposable.email)
      await expect(adminUsers.row(disposable.email)).toBeVisible({ timeout: 15_000 })

      // Action: newly provisioned users start "active". Assert that first.
      await expect(adminUsers.statusBadge(disposable.email)).toHaveText('active')
      await expect(adminUsers.suspendReinstateButton(disposable.email)).toHaveAttribute(
        'title',
        'Suspend'
      )

      // Action: click Suspend -> assert status flips to "suspended" and the
      // toast confirms the mutation, and the button's title flips to "Reinstate".
      await toggleSuspendWithRetry(
        page,
        adminUsers,
        disposable.email,
        `${disposable.email} suspended`
      )
      await expect(adminUsers.statusBadge(disposable.email)).toHaveText('suspended')
      await expect(adminUsers.suspendReinstateButton(disposable.email)).toHaveAttribute(
        'title',
        'Reinstate'
      )

      // Action: click Reinstate -> assert status reverts to "active" (reversible).
      await toggleSuspendWithRetry(
        page,
        adminUsers,
        disposable.email,
        `${disposable.email} reinstated`
      )
      await expect(adminUsers.statusBadge(disposable.email)).toHaveText('active')
      await expect(adminUsers.suspendReinstateButton(disposable.email)).toHaveAttribute(
        'title',
        'Suspend'
      )

      // Action: delete via the admin UI's own delete action (also deletes the
      // underlying Clerk account server-side). Confirm via the dialog.
      await adminUsers.openDeleteDialogFor(disposable.email)
      await expect(adminUsers.deleteDialog(disposable.email)).toBeVisible()
      await adminUsers.confirmDelete(disposable.email)

      // Assertion: success toast fires and the row disappears from the (still
      // email-filtered) list — the strongest available proof the DB row and
      // the Clerk account were both removed.
      await expect(page.getByText(`Deleted ${disposable.email}`)).toBeVisible()
      await expect(adminUsers.row(disposable.email)).toHaveCount(0, { timeout: 15_000 })
    } finally {
      await adminContext.close()
    }
  })
})
