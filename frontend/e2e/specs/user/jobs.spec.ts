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
    // Toggles between two workflow statuses ("Applied" <-> "Interviewing")
    // on an already-tracked job, using the "Tracking" tab (combined view
    // over the 4 workflow stages) so the row never leaves view and the
    // revert path is provably real. (PATCH .../status now also accepts
    // 'new' — see jobs.spec.ts test 5 and useJobActions.ts — but this test
    // doesn't need that path since both statuses here were already
    // writable.)
    const jobsPage = new JobsPage(page)
    await jobsPage.goto()

    await page.getByRole('tab', { name: /^Tracking/ }).click()
    await page.keyboard.press('Escape') // Tracking tab doubles as a Popover trigger; close it if it opened

    const trackingRows = page.getByRole('button', { name: /, status (applied|interviewing)$/ })
    await expect(async () => {
      expect(await trackingRows.count()).toBeGreaterThan(0)
    }).toPass({ timeout: 10_000 })

    const row = trackingRows.first()
    const title = await jobsPage.titleOf(row)
    const company = await jobsPage.companyOf(row)
    const rowLabel = await row.getAttribute('aria-label')
    const originalStatus = rowLabel?.endsWith('applied') ? 'Applied' : 'Interviewing'
    const otherStatus = originalStatus === 'Applied' ? 'Interviewing' : 'Applied'

    // Precondition: the detail panel's status control starts checked on
    // whichever of the two statuses this row currently has.
    await jobsPage.openRow(row)
    await expect(jobsPage.detailStatusRadio(originalStatus)).toHaveAttribute('aria-checked', 'true')

    const stableRow = () => jobsPage.rows().filter({ hasText: title }).filter({ hasText: company })

    // The mutating action + its post-mutation assertions are wrapped in
    // try/finally: if any assertion between changing the status and
    // reverting it throws (the very regression this test exists to catch),
    // the finally block still fires the revert click so the real tracked
    // job is never left permanently changed from its original status.
    try {
      // Action: click the other status radio.
      await jobsPage.detailStatusRadio(otherStatus).click()

      // Assertion: the SAME job's row (matched by title+company, since it
      // stays visible in the combined Tracking view throughout) now
      // announces the new status, and reopening shows that radio checked
      // instead.
      await expect(stableRow()).toHaveAttribute(
        'aria-label',
        new RegExp(`, status ${otherStatus.toLowerCase()}$`),
        { timeout: 10_000 }
      )
      await jobsPage.openRow(stableRow())
      await expect(jobsPage.detailStatusRadio(otherStatus)).toHaveAttribute('aria-checked', 'true')
    } finally {
      // Teardown: idempotent — re-reads the row's CURRENT status rather
      // than assuming the click above landed, and only reverts if it still
      // differs from the original, so a failed first click can't cause this
      // to click past the original status.
      const currentLabel = await stableRow().getAttribute('aria-label')
      if (!currentLabel?.endsWith(`, status ${originalStatus.toLowerCase()}`)) {
        await jobsPage.openRow(stableRow())
        await jobsPage.detailStatusRadio(originalStatus).click()
      }

      // Assertion: the row is back to exactly the status it started with —
      // no residual change to live dev data.
      await expect(stableRow()).toHaveAttribute(
        'aria-label',
        new RegExp(`, status ${originalStatus.toLowerCase()}$`),
        { timeout: 10_000 }
      )
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
    const jobsPage = new JobsPage(page)
    await jobsPage.goto()

    await page.getByRole('tab', { name: /^Tracking/ }).click()
    await page.keyboard.press('Escape') // Tracking tab doubles as a Popover trigger; close it if it opened

    const trackingRows = page.getByRole('button', {
      name: /, status (applied|interviewing|offer|rejected)$/,
    })
    await expect(async () => {
      expect(await trackingRows.count()).toBeGreaterThan(0)
    }).toPass({ timeout: 10_000 })

    const row = trackingRows.first()
    const title = await jobsPage.titleOf(row)
    const company = await jobsPage.companyOf(row)
    const rowLabel = await row.getAttribute('aria-label')
    const originalStatus = rowLabel?.match(/, status (\w+)$/)?.[1]
    if (!originalStatus) throw new Error(`could not parse status from row aria-label: ${rowLabel}`)
    const targetRow = () => jobsPage.rows().filter({ hasText: title }).filter({ hasText: company })

    // The dismiss + undo click are wrapped in try/finally: if undo itself
    // ever regresses again, the finally block still fires a direct restore
    // so this test can never leave a real, pre-existing tracked job
    // permanently dismissed — belt-and-suspenders even though undo is now
    // the primary, verified-working recovery path.
    try {
      // Action: hover the row (reveals the dismiss button, matching real
      // user interaction) then click "Dismiss <title>".
      await row.hover()
      await jobsPage.dismissButton(row, title).click()

      // Assertion: the DismissStripe undo toast appears and names this
      // exact job (message "Dismissed", submessage "<title> at <company>").
      await expect(jobsPage.dismissStripe).toBeVisible()
      await expect(jobsPage.dismissStripe).toContainText('Dismissed')
      await expect(jobsPage.dismissStripe).toContainText(title)

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
      await expect(targetRow()).toHaveAttribute(
        'aria-label',
        new RegExp(`, status ${originalStatus}$`)
      )
    } finally {
      // Safety net only — expected to be a no-op now that undo genuinely
      // works. Re-checks final state and force-restores via the same
      // authenticated endpoint the app itself uses if, for any reason, the
      // row isn't back to its original status.
      const stillWrong =
        (await targetRow().count()) === 0 ||
        !(await targetRow().getAttribute('aria-label'))?.endsWith(`, status ${originalStatus}`)
      if (stillWrong) {
        await jobsPage.openRow(row.or(targetRow()))
        const jobId = new URL(page.url()).searchParams.get('job')
        if (jobId) {
          await page.evaluate(
            async ({ id, status }) => {
              const clerk = (
                window as unknown as {
                  Clerk?: {
                    session?: { getToken: (o: { template: string }) => Promise<string | null> }
                  }
                }
              ).Clerk
              const token = await clerk?.session?.getToken({ template: 'job-radar-jwt-template' })
              await fetch(`/api/jobs/${id}/status`, {
                method: 'PATCH',
                headers: {
                  'Content-Type': 'application/json',
                  ...(token ? { Authorization: `Bearer ${token}` } : {}),
                },
                body: JSON.stringify({ status }),
              })
            },
            { id: jobId, status: originalStatus }
          )
        }
      }
    }
  })
})
