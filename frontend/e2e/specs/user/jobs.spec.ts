import { test, expect } from '@playwright/test'
import { JobsPage } from '../../pages/JobsPage'

// All 5 tests below read/mutate the SAME live "new"-tab jobs feed (shared dev
// data — no per-test fixtures are created). Running them in true parallel
// (Playwright's `fullyParallel: true` default) would let one test's mutation
// (e.g. dismiss) race another test's assertion about row order/count on the
// exact same list. `serial` mode forces them to run one at a time within this
// file so each test starts from — and leaves behind — the same feed state.
test.describe.configure({ mode: 'serial' })

test.describe('Jobs feed', () => {
  test('feed renders with the infinite-scroll sentinel and at least one row', async ({ page }) => {
    const jobsPage = new JobsPage(page)

    // Action: load the Jobs feed (defaults to the "New" tab).
    await jobsPage.goto()

    // Assertion: the infinite-scroll sentinel (data-testid="jobs-table-sentinel")
    // is present and visible, and the table body rendered at least one real
    // job row (each row is a <tr role="button"> whose accessible name always
    // ends in ", status <status>").
    await expect(jobsPage.sentinel).toBeVisible()
    await expect(async () => {
      expect(await jobsPage.rowCount()).toBeGreaterThan(0)
    }).toPass()
  })

  test('clicking a job row opens the detail panel for that exact job', async ({ page }) => {
    const jobsPage = new JobsPage(page)
    await jobsPage.goto()

    const row = jobsPage.firstRow()
    const title = await jobsPage.titleOf(row)
    const company = await jobsPage.companyOf(row)

    // Action: click the first row.
    await jobsPage.openRow(row)

    // Assertion: the always-open detail panel (role="region", name "Job
    // details") appears, its <h2> shows the exact same title text the row
    // itself displayed, and the panel's header also shows the same company
    // name — proving the panel opened for THIS row's job, not a hardcoded one.
    await expect(jobsPage.detailPanel).toBeVisible()
    await expect(jobsPage.detailTitle).toHaveText(title)
    await expect(jobsPage.detailPanel).toContainText(company)
  })

  test('save/unsave toggle flips the row bookmark button and reverts back', async ({ page }) => {
    const jobsPage = new JobsPage(page)
    await jobsPage.goto()

    const row = jobsPage.firstRow()
    const title = await jobsPage.titleOf(row)
    const saveBtn = jobsPage.saveButton(row, title)
    const originalPressed = await saveBtn.getAttribute('aria-pressed')
    const flippedPressed = originalPressed === 'true' ? 'false' : 'true'

    // The revert click (and its assertion) is wrapped in try/finally: if the
    // first toHaveAttribute assertion below throws (the very regression this
    // test exists to catch), the finally block still fires the second click
    // so the real job's saved/bookmarked state is never left flipped from
    // what it was before this test ran.
    try {
      // Action: click "Save <title>" (or "Remove <title> from saved" if it
      // was already saved) once.
      await saveBtn.click()

      // Assertion: the SAME row's bookmark button now reports the opposite
      // aria-pressed state (aria-label flips between "Save {title}" and
      // "Remove {title} from saved" — jobs-columns.tsx bookmark column).
      await expect(jobsPage.saveButton(row, title)).toHaveAttribute('aria-pressed', flippedPressed)
    } finally {
      // Teardown: runs even if the assertion above threw. Idempotent by
      // design — it re-reads the button's CURRENT state (rather than
      // assuming the first click landed) and only clicks if it still
      // differs from the original, so this can't accidentally double-toggle
      // past the original state if the first click never actually took
      // effect (e.g. a network failure before the assertion even ran).
      const currentPressed = await jobsPage.saveButton(row, title).getAttribute('aria-pressed')
      if (currentPressed !== (originalPressed ?? 'false')) {
        await jobsPage.saveButton(row, title).click()
      }

      // Assertion: state is restored to exactly what it was before this test
      // touched it — no residual side effect on live dev data.
      await expect(jobsPage.saveButton(row, title)).toHaveAttribute(
        'aria-pressed',
        originalPressed ?? 'false'
      )
    }
  })

  test('changing status in the detail panel updates the radio and reverting restores it', async ({
    page,
  }) => {
    // Self-seeds a Tracking-tab job from the New tab instead of depending on
    // whatever the live dev account's Tracking tab already contains — that
    // ambient data drained to zero during real usage (the account
    // accumulated 623 dismissed jobs and 0 in any workflow status), which is
    // exactly the fragility this avoids. Promoting New -> Applied -> ... ->
    // New is fully self-contained and reverts completely, now that
    // PATCH .../status accepts 'new' (see useJobActions.ts /
    // JobValidationService.go).
    const jobsPage = new JobsPage(page)
    const seed = await seedTrackedJob(page, jobsPage)
    const stableRow = () =>
      jobsPage.rows().filter({ hasText: seed.title }).filter({ hasText: seed.company })

    await expect(jobsPage.detailStatusRadio('Applied')).toHaveAttribute('aria-checked', 'true')

    // The mutating action + its post-mutation assertions are wrapped in
    // try/finally: if any assertion between changing the status and
    // reverting it throws (the very regression this test exists to catch),
    // the finally block still fires the revert so the job is never left
    // permanently changed — and, since we created it, "changed" here means
    // fully reverted to New, not left at some other tracked status.
    try {
      // Action: click Interviewing.
      await jobsPage.detailStatusRadio('Interviewing').click()

      // Assertion: the same job's row (found via the Tracking tab, since
      // Applied/Interviewing are excluded from New) now announces the new
      // status, and reopening shows that radio checked instead.
      await page.getByRole('tab', { name: /^Tracking/ }).click()
      await page.keyboard.press('Escape') // Tracking tab doubles as a Popover trigger; close it if it opened
      await expect(stableRow()).toHaveAttribute('aria-label', /, status interviewing$/, {
        timeout: 10_000,
      })
      await jobsPage.openRow(stableRow())
      await expect(jobsPage.detailStatusRadio('Interviewing')).toHaveAttribute(
        'aria-checked',
        'true'
      )

      // Action: revert to Applied — proves the revert path is real, not
      // just that Interviewing landed.
      await jobsPage.detailStatusRadio('Applied').click()
      await expect(stableRow()).toHaveAttribute('aria-label', /, status applied$/, {
        timeout: 10_000,
      })
    } finally {
      await revertSeedToNew(page, jobsPage, seed, stableRow)
    }
  })

  test('dismissing a job shows the undo stripe, and clicking undo genuinely restores it', async ({
    page,
  }) => {
    // Undo previously always failed here (a cache-shape bug in
    // useJobActions.ts's lookupStatus/lookupSaved combined with the backend
    // rejecting status='new') — both are fixed now:
    //   - useJobActions.ts scans the real `{ items, next_cursor }` cache
    //     shape instead of a non-existent `.pages` wrapper.
    //   - JobValidationService.SetStatus allows 'new' through, matching what
    //     UserJobStateRepositoryService.Upsert already implemented (clears
    //     the workflow status, leaves saved_at untouched).
    // This test now exercises the REAL, working recovery path end-to-end —
    // no manual authenticated-fetch workaround needed, since Undo itself is
    // the recovery mechanism.
    //
    // Self-seeds its own Tracking-tab job (see the previous test's comment
    // for why) rather than dismissing whatever real job happens to be first
    // in the live account's Tracking tab.
    const jobsPage = new JobsPage(page)
    const seed = await seedTrackedJob(page, jobsPage)
    const targetRow = () =>
      jobsPage.rows().filter({ hasText: seed.title }).filter({ hasText: seed.company })

    await page.getByRole('tab', { name: /^Tracking/ }).click()
    await page.keyboard.press('Escape') // Tracking tab doubles as a Popover trigger; close it if it opened
    await expect(targetRow()).toHaveAttribute('aria-label', /, status applied$/, {
      timeout: 10_000,
    })
    const row = targetRow()

    // The dismiss + undo click are wrapped in try/finally: if undo itself
    // ever regresses again, the finally block still fires a direct restore
    // so this test can never leave the seeded job permanently dismissed —
    // belt-and-suspenders even though undo is now the primary,
    // verified-working recovery path.
    try {
      // Action: hover the row (reveals the dismiss button, matching real
      // user interaction) then click "Dismiss <title>".
      await row.hover()
      await jobsPage.dismissButton(row, seed.title).click()

      // Assertion: the DismissStripe undo toast appears and names this
      // exact job (message "Dismissed", submessage "<title> at <company>").
      await expect(jobsPage.dismissStripe).toBeVisible()
      await expect(jobsPage.dismissStripe).toContainText('Dismissed')
      await expect(jobsPage.dismissStripe).toContainText(seed.title)

      // Assertion: the row is gone from the Tracking-tab feed (dismissed
      // jobs are excluded from the 4 workflow-stage statuses).
      await expect(targetRow()).toHaveCount(0, { timeout: 10_000 })

      // Action: click Undo on the stripe (scoped to the stripe's own testid
      // to avoid Sonner's second, generic "Undo" toast fired by the same
      // mutation) — let the mutation's onSuccess/pushHistory land first so
      // this isn't racing the async dismiss call itself.
      await page.waitForTimeout(1000)
      await jobsPage.dismissStripeUndoButton.click()

      // Assertion: undo genuinely succeeds — a success toast, not "Undo
      // failed" — and the job reappears with its exact original status.
      // This is the real, live-verified fix: previously this always showed
      // "Undo failed" and the row stayed absent.
      await expect(page.getByText('Undone', { exact: true })).toBeVisible({ timeout: 10_000 })
      await expect(targetRow()).toHaveCount(1, { timeout: 10_000 })
      await expect(targetRow()).toHaveAttribute('aria-label', /, status applied$/)
    } finally {
      await revertSeedToNew(page, jobsPage, seed, targetRow)
    }
  })
})

// ── Shared seed/revert helpers for tests 4 and 5 ────────────────────────────
// Both self-seed a Tracking-tab job from the New tab (see each test's own
// comment for why) and must fully revert it back to New afterward — this
// logic is identical in both, so it's factored out rather than duplicated.

interface SeededJob {
  title: string
  company: string
  jobId: string | null
}

/**
 * Promotes the first New-tab row to "Applied", returning its identity.
 *
 * Real, live-verified app bug found while writing this: JobDetailPanel
 * receives its `job` as a prop looked up by ID from the currently-active
 * list query's cache (not fetched independently) — so changing a New job's
 * status while still viewing the New tab moves it out of that tab's own
 * list on invalidation/refetch, and the still-open panel never re-syncs
 * (confirmed live: the PATCH returns 200, but the radio's aria-checked
 * stays "false" indefinitely). The original version of this test never hit
 * this because it only ever toggled between two Tracking-tab statuses while
 * viewing the Tracking tab, which shows all 4 workflow stages together —
 * the job never disappeared from the currently-viewed list. Documented in
 * frontend/e2e/CLAUDE.md rather than fixed here (out of scope for this
 * change); worked around below by switching to the Tracking tab and
 * re-opening the row there — reading fresh data instead of trusting the
 * stale, still-open New-tab panel instance, same as a real user would need
 * to do to see the update reflected.
 */
async function seedTrackedJob(
  page: import('@playwright/test').Page,
  jobsPage: JobsPage
): Promise<SeededJob> {
  await jobsPage.goto()
  const row = jobsPage.firstRow()
  const title = await jobsPage.titleOf(row)
  const company = await jobsPage.companyOf(row)

  await jobsPage.openRow(row)
  const jobId = new URL(page.url()).searchParams.get('job')

  // Wait for the panel to be fully open and showing THIS job (not just
  // "some detail panel") before interacting with its status control —
  // clicking immediately after openRow() raced the panel's own data fetch
  // often enough to be a real, reproducible flake (the click landed before
  // the radio group's mutation handler was wired up, so aria-checked never
  // flipped).
  await expect(jobsPage.detailPanel).toBeVisible()
  await expect(jobsPage.detailTitle).toHaveText(title)
  await expect(jobsPage.detailStatusRadio('New')).toHaveAttribute('aria-checked', 'true')

  await jobsPage.detailStatusRadio('Applied').click()

  // Don't trust the still-open New-tab panel instance (see doc comment
  // above) — switch to Tracking and re-open the row there for fresh data.
  const stableRow = () => jobsPage.rows().filter({ hasText: title }).filter({ hasText: company })
  await page.getByRole('tab', { name: /^Tracking/ }).click()
  await page.keyboard.press('Escape')
  await expect(stableRow()).toHaveAttribute('aria-label', /, status applied$/, {
    timeout: 10_000,
  })
  await jobsPage.openRow(stableRow())
  await expect(jobsPage.detailStatusRadio('Applied')).toHaveAttribute('aria-checked', 'true')

  return { title, company, jobId }
}

/**
 * Reverts a job seeded by seedTrackedJob() back to New. Tries the real UI
 * path first (find it via the Tracking tab, click the New radio); falls
 * back to a direct authenticated PATCH (same pattern as the rest of this
 * suite's safety nets) if the UI path can't find or revert it — e.g. undo
 * left it dismissed, or an assertion threw before the row was locatable.
 */
async function revertSeedToNew(
  page: import('@playwright/test').Page,
  jobsPage: JobsPage,
  seed: SeededJob,
  findRow: () => import('@playwright/test').Locator
) {
  await page.getByRole('tab', { name: /^Tracking/ }).click()
  await page.keyboard.press('Escape')
  const count = await findRow().count()
  if (count > 0) {
    await jobsPage.openRow(findRow())
    const current = await jobsPage.detailStatusRadio('New').getAttribute('aria-checked')
    if (current !== 'true') {
      await jobsPage.detailStatusRadio('New').click()
      // Same stale-list app bug documented on seedTrackedJob: moving a job
      // OUT of the currently-viewed tab's filter (here, Tracking losing a
      // job that just went back to New) doesn't live-refresh the list —
      // confirmed live (PATCH 200, row still rendered 20+ polls over 10s).
      // Reload rather than trust in-session cache reactivity, same pattern
      // as this suite's other direct-restore fallbacks.
      await page.waitForTimeout(500)
      await page.reload()
      await page.getByRole('tab', { name: /^Tracking/ }).click()
      await page.keyboard.press('Escape')
    }
    await expect(findRow()).toHaveCount(0, { timeout: 10_000 }) // gone from Tracking once back to New
    return
  }

  // Not found via the UI (e.g. still dismissed) — force it directly.
  if (seed.jobId) {
    await page.evaluate(
      async ({ id }) => {
        const clerk = (
          window as unknown as {
            Clerk?: { session?: { getToken: (o: { template: string }) => Promise<string | null> } }
          }
        ).Clerk
        const token = await clerk?.session?.getToken({ template: 'job-radar-jwt-template' })
        await fetch(`/api/jobs/${id}/status`, {
          method: 'PATCH',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ status: 'new' }),
        })
      },
      { id: seed.jobId }
    )
  }
}
