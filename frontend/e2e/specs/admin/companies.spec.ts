import { test, expect } from '@playwright/test'
import { CompaniesPage } from '../../pages/admin/CompaniesPage'

// Fixed-URL adapter per AddCompanyModal.tsx ADAPTER_HINTS + the `ats` table
// seed (migration 077_ats_table.sql): base_url = '' so no careers URL input
// is required to submit the form.
const FIXED_URL_ADAPTER = 'google'

test.describe('Admin Companies page', () => {
  test('creates a fixed-URL-adapter company and then permanently deletes it', async ({ page }) => {
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 10000)}`
    const companyId = `e2e-test-co-${suffix}`
    const companyName = `E2E Test Co ${suffix}`

    const companies = new CompaniesPage(page)
    await companies.goto()

    // The catalog has 1000+ companies (default page size 20, no default
    // sort guarantees a fresh row lands on page 1), so every lookup below
    // searches by the unique disposable name to bring the row onto the
    // single visible page rather than relying on pagination.
    await companies.searchFor(companyName)

    // Precondition: the disposable company does not exist yet.
    await expect(companies.row(companyName)).toHaveCount(0)

    // Action: open "Add company", pick the fixed-URL adapter (google — no
    // careers URL required), fill the minimum fields, and submit.
    await companies.addFixedUrlCompany({
      adapter: FIXED_URL_ADAPTER,
      id: companyId,
      name: companyName,
    })

    // Assertion: the new company now appears in the companies table, listed
    // under its chosen adapter. The search box still holds the same filter
    // from the precondition check, so the list query re-runs and surfaces
    // the newly created row.
    const createdRow = companies.row(companyName)
    await expect(createdRow).toBeVisible({ timeout: 10_000 })
    await expect(createdRow).toContainText('Google')

    // Action: delete that exact company via its own row delete action,
    // confirming the "permanently delete... cannot be undone" dialog.
    await companies.openDeleteDialogFor(companyName)
    await expect(companies.deleteDialog()).toBeVisible()
    await expect(companies.deleteDialog()).toContainText(companyName)
    await expect(companies.deleteDialog()).toContainText('cannot be undone')
    await companies.confirmDelete()

    // Assertion: the company no longer appears in the list.
    await expect(companies.row(companyName)).toHaveCount(0, { timeout: 10_000 })
  })
})
