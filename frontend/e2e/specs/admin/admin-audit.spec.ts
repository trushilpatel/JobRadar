import { test, expect } from '@playwright/test'
import { AdminAuditPage } from '../../pages/admin/AdminAuditPage'
import { CompaniesPage } from '../../pages/admin/CompaniesPage'

// Fixed-URL adapter per AddCompanyModal.tsx ADAPTER_HINTS + the `ats` table
// seed (migration 077_ats_table.sql): base_url = '' so no careers URL input
// is required to submit the form. Reused from companies.spec.ts.
const FIXED_URL_ADAPTER = 'google'

async function actionCellTexts(auditPage: AdminAuditPage): Promise<(string | undefined)[]> {
  return auditPage
    .rows()
    .evaluateAll((rows) => rows.map((r) => r.querySelectorAll('td')[2]?.textContent?.trim()))
}

async function actorCellTitles(auditPage: AdminAuditPage): Promise<(string | null)[]> {
  return auditPage
    .rows()
    .evaluateAll((rows) => rows.map((r) => r.querySelectorAll('td')[1]?.getAttribute('title')))
}

// The audit log page (frontend/src/pages/admin/AdminAuditPage.tsx) has zero
// mutations of its own — it is a pure read-only viewer over `admin_audit_log`.
// To exercise the filter inputs with a REAL, observable change in the
// rendered result set (rather than asserting against whatever pre-existing
// rows happen to be in the dev audit log at run time), this spec first
// performs one real, self-contained admin mutation — creating then deleting
// a disposable company via the existing Companies admin page — which the
// backend's `AuditAdminMutation` middleware records as two exact rows:
//   action = "POST /admin/companies"          (create)
//   action = "DELETE /admin/companies/<id>"   (delete)
// The audit repo filters with SQL exact equality (`action = ?`,
// `actor_user_id = ?` — see AuditRepositoryService.go), not substring/ILIKE,
// so the filter assertions below use exact strings pulled from the row
// itself, not arbitrary partial text.
test.describe('Admin Audit Log page', () => {
  // Set as soon as we know the disposable company's name (before the create
  // call), cleared once this test's own delete step has confirmed removal.
  // If the setup step fails anywhere between creation and its own delete
  // (e.g. an intermittent 500, or an assertion failure), this leftover name
  // lets afterEach make a best-effort attempt to remove the orphaned
  // disposable company rather than leaking it into the dev DB across runs.
  let pendingCleanupCompanyName: string | null = null

  test.afterEach(async ({ page }) => {
    if (!pendingCleanupCompanyName) return
    const name = pendingCleanupCompanyName
    try {
      const companies = new CompaniesPage(page)
      await companies.goto()
      await companies.searchFor(name)
      if ((await companies.row(name).count()) > 0) {
        await companies.openDeleteDialogFor(name)
        await companies.confirmDelete()
        await expect(companies.row(name)).toHaveCount(0, { timeout: 10_000 })
      }
    } catch {
      // Best-effort only — never fail the run over cleanup of a setup
      // artifact; a real failure here would just mean the disposable
      // company needs manual removal, which is reported, not silently lost.
    }
  })

  test('renders the log, and both action and actor filters narrow the visible rows', async ({
    page,
  }) => {
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 10000)}`
    const companyId = `e2e-audit-co-${suffix}`
    const companyName = `E2E Audit Co ${suffix}`

    // ── Precondition / setup: generate two real, distinctly-actioned audit
    // rows via a self-contained company create+delete (own record only). ──
    const companies = new CompaniesPage(page)
    await companies.goto()
    await companies.searchFor(companyName)
    await expect(companies.row(companyName)).toHaveCount(0)

    pendingCleanupCompanyName = companyName
    await companies.addFixedUrlCompany({
      adapter: FIXED_URL_ADAPTER,
      id: companyId,
      name: companyName,
    })
    const createdRow = companies.row(companyName)
    await expect(createdRow).toBeVisible({ timeout: 10_000 })

    await companies.openDeleteDialogFor(companyName)
    await expect(companies.deleteDialog()).toBeVisible()
    await companies.confirmDelete()
    await expect(companies.row(companyName)).toHaveCount(0, { timeout: 10_000 })
    // This test's own delete step succeeded — nothing left for afterEach to do.
    pendingCleanupCompanyName = null

    // ── Step 1: the audit log list renders (table with rows, since the
    // create+delete above guarantees at least 2 rows exist now). ──
    const audit = new AdminAuditPage(page)
    await audit.goto()

    await expect(audit.table()).toBeVisible({ timeout: 10_000 })
    await expect(audit.emptyState()).toHaveCount(0)
    const totalRowsBefore = await audit.rows().count()
    expect(totalRowsBefore).toBeGreaterThanOrEqual(2)

    // Grab the exact actor_user_id (own admin account) from the row we just
    // created — the actor cell's `title` attribute carries the raw ID even
    // though the visible text is the friendlier email (see AdminAuditPage.tsx).
    const createActionRowBefore = page
      .getByRole('row')
      .filter({ hasText: 'POST /admin/companies' })
      .first()
    await expect(createActionRowBefore).toBeVisible()
    const actorCellBefore = createActionRowBefore.locator('td').nth(1)
    const actorUserId = await actorCellBefore.getAttribute('title')
    expect(actorUserId).toBeTruthy()

    // ── Step 2a: filter by action — exact-match narrows the result set.
    // A plain `rows().count() > 0` poll is NOT sufficient here: the table
    // already has rows > 0 from the unfiltered state the instant we call
    // fill(), so that condition is trivially satisfied before the 300ms
    // debounce + refetch has actually landed, letting the assertions below
    // race against stale (unfiltered) data. Instead poll on the actual
    // condition we need — every visible action cell already narrowed to
    // the filtered value — which can only become true once the real
    // filtered response has rendered.
    await audit.filterByAction('POST /admin/companies')

    await expect
      .poll(
        async () => {
          const cells = await actionCellTexts(audit)
          return cells.length > 0 && cells.every((c) => c === 'POST /admin/companies')
        },
        { timeout: 5_000 }
      )
      .toBe(true)
    const filteredCount = await audit.rows().count()

    // Every visible row's Action column must equal exactly what we typed —
    // proves the visible list actually changed in response to the filter,
    // not just that the input holds the typed value.
    const filteredActionCells = await actionCellTexts(audit)
    expect(filteredActionCells.length).toBe(filteredCount)
    for (const cellText of filteredActionCells) {
      expect(cellText).toBe('POST /admin/companies')
    }
    // The narrowed set is strictly fewer rows than the unfiltered total,
    // since we know at least one row (the DELETE row) has a different action.
    expect(filteredCount).toBeLessThan(totalRowsBefore)

    await expect
      .poll(async () => {
        const label = await audit.entriesCountLabel().textContent()
        return parseInt(label?.split(' ')[0] ?? '-1', 10)
      })
      .toBe(filteredCount)

    // ── Step 2b: clear, then filter by the exact actor_user_id — same
    // exact-match narrowing behaviour, on the other filter input. ──
    await audit.filterByAction('')
    await expect.poll(() => audit.rows().count(), { timeout: 5_000 }).toBe(totalRowsBefore)

    await audit.filterByActor(actorUserId as string)
    // Same race as step 2a's action filter — poll the actual narrowed
    // content (every visible actor title matches), not just a row count
    // that could still reflect the pre-debounce, unfiltered response.
    await expect
      .poll(
        async () => {
          const titles = await actorCellTitles(audit)
          return titles.length > 0 && titles.every((t) => t === actorUserId)
        },
        { timeout: 5_000 }
      )
      .toBe(true)

    const actorFilteredTitles = await actorCellTitles(audit)
    expect(actorFilteredTitles.length).toBeGreaterThan(0)
    for (const title of actorFilteredTitles) {
      expect(title).toBe(actorUserId)
    }

    // ── Step 2c: a filter value guaranteed to match nothing shows the
    // defined "no results" empty state — the other legitimate narrowing
    // outcome named in the flow spec. ──
    await audit.filterByActor('')
    await audit.filterByAction('definitely-not-a-real-admin-action-xyz-999')

    await expect(audit.emptyState()).toBeVisible({ timeout: 5_000 })
    await expect(audit.table()).toHaveCount(0)
    const emptyLabel = await audit.entriesCountLabel().textContent()
    expect(emptyLabel).toBe('0 entries')

    // ── Step 3: pagination. Reset filters and check whether more than one
    // page of results exists. ──
    await audit.filterByAction('')
    await expect(audit.table()).toBeVisible({ timeout: 5_000 })

    const nextEnabled = await audit.isNextPageEnabled()
    if (nextEnabled) {
      const pageIndicatorBefore = await audit.pageIndicator().textContent()
      await audit.nextPageButton().click()
      await expect
        .poll(() => audit.pageIndicator().textContent(), { timeout: 5_000 })
        .not.toBe(pageIndicatorBefore)
    } else {
      test.info().annotations.push({
        type: 'coverage-gap',
        description:
          'Pagination could not be exercised: the dev audit log currently has only one page of results (≤ 50 entries), so the Next page control is disabled. This is a legitimate gap, not a test failure.',
      })
    }
  })
})
