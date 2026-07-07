import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from '@playwright/test'
import { ResumeLibraryPage } from '../../pages/ResumeLibraryPage'

const dirname = path.dirname(fileURLToPath(import.meta.url))
const SAMPLE_RESUME_PATH = path.join(dirname, '../../fixtures/sample-resume.pdf')

test.describe('Resume Library', () => {
  test('uploading a resume adds it to the list, and deleting it removes it again', async ({
    page,
  }) => {
    const resumeLibrary = new ResumeLibraryPage(page)
    // Unique per test run so this test can never match a resume left behind
    // by a previous (e.g. failed) run — every assertion and the final
    // delete target this exact label, never any pre-existing resume.
    const uniqueLabel = `e2e-resume-${Date.now()}`

    await resumeLibrary.goto()

    // NOTE: this deliberately does NOT assert on the library's total resume
    // count. apply-queue.spec.ts uploads/deletes its own seed resume on this
    // same shared TEST_ROLE_USER account concurrently (both specs run under
    // the chromium-user project), so a total-count delta assertion here is a
    // real, observed cross-spec race — an unrelated concurrent upload/delete
    // can land between this test's "before" and "after" reads and throw the
    // exact count off by one. Asserting on this test's own uniquely-labeled
    // card (never a value any other spec could produce) proves the same
    // behavior without depending on the library being quiescent.

    // Action: open the upload dialog and submit the sample PDF fixture with
    // a unique label.
    await resumeLibrary.openUploadDialog()
    await resumeLibrary.uploadResume(SAMPLE_RESUME_PATH, uniqueLabel)

    // Assertion: a card carrying both the unique label and the uploaded
    // file's original filename is visible — proves the upload round-tripped
    // through the backend, not just that the dialog closed.
    const uploadedCard = resumeLibrary.cardByLabel(uniqueLabel)
    await expect(uploadedCard).toBeVisible()
    await expect(uploadedCard).toContainText('sample-resume.pdf')

    // Action: delete the exact resume this test just created via its
    // ResumeCard's Delete button.
    await resumeLibrary.deleteCard(uniqueLabel)

    // Assertion: this exact card is gone — no other (pre-existing, or
    // concurrently-created-by-another-spec) resume was touched.
    await expect(resumeLibrary.cardByLabel(uniqueLabel)).toHaveCount(0)
  })
})
