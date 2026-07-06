import { test, expect } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { AccountPage } from '../../pages/AccountPage'
import {
  createDisposableClerkUser,
  deleteDisposableClerkUser,
} from '../../fixtures/disposable-clerk-user'

test.describe('Account page', () => {
  test('edit display name', async ({ page }) => {
    const account = new AccountPage(page)
    await account.goto()

    const original = await account.readDisplayName()
    const tempName = `E2E Temp ${randomUUID().slice(0, 8)}`

    // Change to a temporary value and save.
    await account.setDisplayName(tempName)
    await account.save()

    // Success toast confirms the mutation succeeded.
    await expect(page.getByText('Profile updated.')).toBeVisible()
    // Field reflects the new value after the identity refetch.
    await expect(account.displayNameInput()).toHaveValue(tempName)

    // Restore the original value so the shared test account is unaffected.
    await account.setDisplayName(original)
    await account.save()
    await expect(page.getByText('Profile updated.')).toBeVisible()
    await expect(account.displayNameInput()).toHaveValue(original)
  })

  test('delete account', async ({ browser }) => {
    // The flow includes two real Clerk redirects (sign-in -> /jobs, then
    // post-delete sign-out -> /login), each with its own up-to-15s wait —
    // the default 30s test timeout doesn't leave enough headroom.
    test.setTimeout(60_000)

    const disposable = await createDisposableClerkUser('user')

    // Playwright merges the project's `use` config (storageState: user.json)
    // into any browser.newContext() call by default — override it explicitly
    // so this context starts truly signed out, not as TEST_ROLE_USER.
    const context = await browser.newContext({ storageState: undefined })
    const page = await context.newPage()

    try {
      // Sign in as the disposable user via the real Clerk UI (verified sequence).
      await page.goto('/login')
      await page.locator('input[name="identifier"]').fill(disposable.email)
      await page.locator('input[name="password"]').fill(disposable.password)
      await page.getByRole('button', { name: 'Continue', exact: true }).click()
      await page.waitForTimeout(1000)
      await page.keyboard.type(disposable.otp, { delay: 50 })
      await expect(page).toHaveURL(/\/jobs/, { timeout: 15_000 })

      const account = new AccountPage(page)
      await account.goto()

      // Trigger the delete-account flow and confirm via the dialog.
      await account.openDeleteDialog()
      await expect(page.getByText('Delete account?')).toBeVisible()
      await account.confirmDelete()

      // deleteMe() + Clerk signOut() -> RequireAuth sees isSignedIn === false
      // and redirects to /login.
      await expect(page).toHaveURL(/\/login/, { timeout: 15_000 })
    } finally {
      await context.close()
      // Safety-net cleanup — no-op (404) if the UI flow already deleted the user.
      await deleteDisposableClerkUser(disposable.clerkUserId)
    }
  })
})
