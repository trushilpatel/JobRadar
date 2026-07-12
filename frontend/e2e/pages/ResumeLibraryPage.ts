import { type Page, type Locator, expect } from '@playwright/test'

/**
 * Page Object for the Resume Library page (/resumes — user-facing despite
 * living under src/pages/admin/ResumeLibraryPage.tsx).
 *
 * There are no data-testid hooks on this page, so every locator here is
 * derived from real accessible names read from the component source:
 * ResumeLibraryPage.tsx ("Upload resume" button, "Resumes" heading),
 * UploadResumeDialog.tsx (dialog role, "File (PDF or DOCX, up to 2MB)" /
 * "Label" field labels, "Upload" submit button), and ResumeCard.tsx
 * (aria-label="Delete" on the per-card delete button).
 */
export class ResumeLibraryPage {
  readonly page: Page

  constructor(page: Page) {
    this.page = page
  }

  async goto() {
    await this.page.goto('/resumes')
    await expect(this.page.getByRole('heading', { name: 'Resumes' })).toBeVisible()
  }

  get uploadResumeButton(): Locator {
    // Header button — always rendered (distinct from the empty-state's
    // "Upload first resume" button, matched via exact text).
    return this.page.getByRole('button', { name: 'Upload resume', exact: true })
  }

  get dialog(): Locator {
    return this.page.getByRole('dialog')
  }

  get fileInput(): Locator {
    return this.dialog.getByLabel('File (PDF or DOCX, up to 2MB)')
  }

  get labelInput(): Locator {
    return this.dialog.getByLabel('Label')
  }

  get dialogSubmitButton(): Locator {
    return this.dialog.getByRole('button', { name: 'Upload', exact: true })
  }

  async openUploadDialog() {
    await this.uploadResumeButton.click()
    await expect(this.dialog).toBeVisible()
  }

  /**
   * Selects the given file, overwrites the auto-filled label with a caller
   * supplied unique label (so the resulting card can be targeted
   * unambiguously later, even if a same-named file lingers from a prior
   * run), and submits.
   */
  async uploadResume(filePath: string, uniqueLabel: string) {
    await this.fileInput.setInputFiles(filePath)
    await this.labelInput.fill(uniqueLabel)
    await this.dialogSubmitButton.click()
    await expect(this.dialog).toBeHidden()
  }

  /** Number of resume cards currently rendered — each card has exactly one Delete button. */
  async resumeCount(): Promise<number> {
    return this.page.getByRole('button', { name: 'Delete' }).count()
  }

  /**
   * Locates the single ResumeCard root element whose subtree contains the
   * given (unique) label text and a Delete button. Selecting via `div` +
   * `.last()` after a two-stage `filter` picks the innermost matching
   * element, i.e. the ResumeCard's own root div, not an outer wrapper —
   * Playwright resolves `locator('div')` in document order, and ancestors
   * always appear before descendants in document order.
   */
  cardByLabel(uniqueLabel: string): Locator {
    return this.page
      .locator('div')
      .filter({ hasText: uniqueLabel })
      .filter({ has: this.page.getByRole('button', { name: 'Delete' }) })
      .last()
  }

  async deleteCard(uniqueLabel: string) {
    const card = this.cardByLabel(uniqueLabel)
    await card.getByRole('button', { name: 'Delete' }).click()
  }
}
