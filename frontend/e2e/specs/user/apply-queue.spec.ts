import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page } from '@playwright/test'
import { ApplyQueuePage } from '../../pages/ApplyQueuePage'
import { ResumeLibraryPage } from '../../pages/ResumeLibraryPage'

const dirname = path.dirname(fileURLToPath(import.meta.url))
const SAMPLE_RESUME_PATH = path.join(dirname, '../../fixtures/sample-resume.pdf')

interface SeededResume {
  id: string
  label: string
}

// The upload response is either the raw Resume object or a `{data: Resume}`
// envelope (see lib/core/api-client.ts's `unwrap`) — tolerate both instead of
// assuming one shape.
interface UploadResumeResponseBody {
  data?: { id?: string; label?: string }
  id?: string
  label?: string
}

/**
 * Uploads one resume with a brand-new, run-scoped unique label and returns
 * its server-assigned id + label (read from the real POST /resumes response,
 * not assumed from the UI). Used to seed the Apply Queue resume-picker test
 * with a resume whose label is guaranteed to have never existed before this
 * test ran — see the two comments at its call site for why that matters.
 */
async function uploadSeedResume(
  page: Page,
  resumeLibrary: ResumeLibraryPage
): Promise<SeededResume> {
  const label = `e2e-apply-queue-${Date.now()}`
  await resumeLibrary.goto()
  await resumeLibrary.openUploadDialog()
  await resumeLibrary.fileInput.setInputFiles(SAMPLE_RESUME_PATH)
  await resumeLibrary.labelInput.fill(label)
  const [uploadResponse] = await Promise.all([
    page.waitForResponse(
      (res) => res.url().includes('/resumes') && res.request().method() === 'POST'
    ),
    resumeLibrary.dialogSubmitButton.click(),
  ])
  await expect(resumeLibrary.dialog).toBeHidden()

  const body = (await uploadResponse.json()) as UploadResumeResponseBody
  const created = body.data ?? body
  if (!created.id) throw new Error('Seed resume upload response did not include an id')
  return { id: created.id, label: created.label ?? label }
}

test.describe('Apply Queue page', () => {
  test('detail panel opens for a queued job, and the resume picker attaches/clears a resume', async ({
    page,
  }) => {
    const applyQueue = new ApplyQueuePage(page)
    const resumeLibrary = new ResumeLibraryPage(page)

    // The query starts in a loading state (skeleton rows, no accessible rows
    // or empty-state text yet) — wait for whichever real outcome settles in
    // before counting, so we don't mistake "still loading" for "empty".
    await applyQueue.goto()
    const emptyMessage = applyQueue.emptyState('No jobs in your apply queue yet')
    await expect(applyQueue.rows().first().or(emptyMessage)).toBeVisible()

    const rowCount = await applyQueue.rowCount()

    // ── Empty queue: assert the empty state and stop — a valid outcome. ────
    if (rowCount === 0) {
      await expect(emptyMessage).toBeVisible()
      return
    }

    // ── Seed exactly one fresh, uniquely-labeled resume up front. ───────────
    // This runs unconditionally (never gated on "does the library already
    // have resumes") for two reasons, both driven by a real review finding
    // against this spec:
    //
    // 1. The shared TEST_ROLE_USER dev account's resume count is live,
    //    mutable data — it has been observed at 0 and at 1+ at different
    //    times. Gating the attach/verify/revert path on "if resumes already
    //    exist" let the test silently degrade to the trivial "no resumes
    //    uploaded yet" early exit with no visible signal in a green CI run.
    //    Seeding here guarantees the meaningful path always executes.
    // 2. The seeded label (`e2e-apply-queue-<timestamp>`) has never existed
    //    before this exact test run, so it cannot coincidentally match any
    //    pre-existing resume's label or the client-side "best guess"
    //    auto-pick label the table can show for a job with no explicit
    //    attachment (see pages/apply-queue/useApplyQueueResumeMatch.ts).
    //    Attaching THIS resume and observing its exact label appear makes
    //    the later "attached" assertion airtight instead of relying on
    //    display text that could coincide with an auto-pick by chance.
    let seededResume: SeededResume | null = null
    try {
      seededResume = await uploadSeedResume(page, resumeLibrary)

      // ── Re-open the Apply Queue and the first queued item's detail panel. ──
      await applyQueue.goto()
      await expect(applyQueue.rows().first()).toBeVisible()
      const row = applyQueue.firstRow()
      await row.click()
      await expect(applyQueue.detailPanel()).toBeVisible()

      const resumeTrigger = applyQueue.resumeTriggerFor(row)
      await expect(resumeTrigger).toBeVisible()
      // Captured before touching anything — the table shows this row's
      // "effective" resume, which can be a display-only best-guess auto-pick
      // even when nothing is explicitly attached. We use it purely to
      // assert we've put the row back exactly how we found it at the end.
      const originalTriggerText = (await resumeTrigger.innerText()).trim()

      // ── Open the resume picker. ────────────────────────────────────────────
      await resumeTrigger.click()
      await expect(applyQueue.dialog()).toBeVisible()

      // We just guaranteed at least one resume exists, so the dialog's own
      // "no resumes" empty state must never appear here. Assert that
      // explicitly instead of silently branching around it, so a real
      // regression (e.g. resumes not scoped correctly to this user, or the
      // seed upload silently failing) fails loudly instead of hiding behind
      // a "valid" early exit.
      await expect(applyQueue.dialogEmptyState()).toBeHidden()

      // "Clear selection" only renders when the job already has a real
      // (explicit) resume_id attached — this is the true signal of prior
      // attachment, unlike the trigger button text, which can show an
      // auto-picked label even with no explicit attachment.
      const hadExplicitAttachment = await applyQueue.clearSelectionButton().isVisible()

      // If a resume was already explicitly attached to this real queue item,
      // remember which row that was so we can re-select it at the end
      // instead of clearing — clearing would leave real dev data in a
      // different state than we found it.
      let originallyAttachedLabel: string | null = null
      if (hadExplicitAttachment) {
        const highlighted = applyQueue.dialog().locator('tr[class*="accent-dim"]')
        await expect(highlighted).toHaveCount(1)
        originallyAttachedLabel = (
          await highlighted.locator('span.font-medium').first().innerText()
        ).trim()
      }

      // ── Select the freshly seeded resume and attach it. ────────────────────
      const seededOption = applyQueue.resumeOptionRow(seededResume.label)
      await expect(seededOption).toBeVisible()
      await seededOption.click()

      const [attachRequest] = await Promise.all([
        page.waitForRequest(
          (req) =>
            req.url().includes('/apply-queue/') &&
            req.url().endsWith('/resume') &&
            req.method() === 'PUT'
        ),
        applyQueue.useThisResumeButton().click(),
      ])
      // Mutation-specific proof the write went through with exactly this
      // resume: the real PUT request body carries the seeded resume's
      // server-assigned id. This is immune to the display-text coincidence
      // called out in review (an auto-picked label happening to equal the
      // top-ranked option's label even if the mutation silently no-ops) —
      // it inspects the actual network call, not the UI's redraw of it.
      expect(attachRequest.postDataJSON()).toEqual({ resume_id: seededResume.id })

      await expect(applyQueue.dialog()).toBeHidden()
      await expect(page.getByText('Resume updated').first()).toBeVisible()

      // Attached: the row's resume trigger now shows the seeded resume's
      // label — a label that provably did not exist anywhere before this
      // test run, so this cannot be a coincidental auto-pick match.
      await expect(resumeTrigger).toContainText(seededResume.label)

      // ── Revert, restoring this row to exactly how we found it. ─────────────
      await resumeTrigger.click()
      await expect(applyQueue.dialog()).toBeVisible()
      // Now that a resume_id is set, "Clear selection" is always available.
      await expect(applyQueue.clearSelectionButton()).toBeVisible()

      if (hadExplicitAttachment && originallyAttachedLabel) {
        // Re-select the original explicit attachment rather than clearing it away.
        await applyQueue.dialog().getByRole('row', { name: originallyAttachedLabel }).click()
        await applyQueue.useThisResumeButton().click()
      } else {
        // Nothing was explicitly attached before — clear reverts to that state.
        await applyQueue.clearSelectionButton().click()
      }

      await expect(applyQueue.dialog()).toBeHidden()
      await expect(page.getByText('Resume updated').first()).toBeVisible()
      await expect(resumeTrigger).toHaveText(originalTriggerText)

      await applyQueue.closeDetailButton().click()
      await expect(applyQueue.detailPanel()).toBeHidden()
    } finally {
      // ── Teardown: always delete the resume this test created, regardless
      // of which branch above ran (including an assertion failure) — the
      // shared resume library is never left holding synthetic e2e data.
      if (seededResume) {
        const seeded = seededResume
        await resumeLibrary.goto()
        await resumeLibrary.deleteCard(seeded.label)
        await expect(resumeLibrary.cardByLabel(seeded.label)).toHaveCount(0)
      }
    }
  })
})
