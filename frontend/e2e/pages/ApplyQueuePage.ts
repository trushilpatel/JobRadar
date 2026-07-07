import type { Page, Locator } from '@playwright/test'

/**
 * Page Object for the Apply Queue page
 * (frontend/src/pages/ApplyQueuePage.tsx) and the resume-picker dialog it
 * opens (frontend/src/features/apply-queue/components/ResumePickerDialog.tsx).
 *
 * Locator notes:
 * - Queue rows render as `<tr role="button" aria-label="{title} at {company},
 *   status {status}">` (see ApplyQueuePage's rowLabel + getRowProps wiring),
 *   so every row is reachable as an accessible button whose name always
 *   contains ", status ".
 * - The per-row "Select resume" trigger is a real nested <button> inside that
 *   <tr>, with aria-label `Select resume for {job_id}` (a UUID) — matched
 *   here by the stable "Select resume for" prefix instead of the id.
 * - The resume-picker dialog's own option rows do NOT get a getRowProps
 *   override, so they keep the table's native implicit `row` role — matched
 *   by their visible resume label text.
 */
export class ApplyQueuePage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/apply-queue')
  }

  emptyState(message: string): Locator {
    return this.page.getByText(message, { exact: true })
  }

  /** All queue rows — each is a `<tr role="button">` announcing title/company/status. */
  rows(): Locator {
    return this.page.getByRole('button', { name: /, status (new|saved|applied)$/ })
  }

  async rowCount(): Promise<number> {
    return this.rows().count()
  }

  firstRow(): Locator {
    return this.rows().first()
  }

  async openRow(row: Locator) {
    await row.click()
  }

  resumeTriggerFor(row: Locator): Locator {
    return row.getByRole('button', { name: /^Select resume for/ })
  }

  // ── Job detail panel (features/jobs/components/JobDetailPanel.tsx) ────────

  detailPanel(): Locator {
    return this.page.getByRole('region', { name: 'Job details' })
  }

  closeDetailButton(): Locator {
    return this.page.getByRole('button', { name: 'Close job details' })
  }

  // ── Resume picker dialog ────────────────────────────────────────────────

  dialog(): Locator {
    return this.page.getByRole('dialog')
  }

  dialogEmptyState(): Locator {
    return this.dialog().getByText('No resumes uploaded yet — add one from the Resume Library.')
  }

  resumeOptionRow(label: string): Locator {
    return this.dialog().getByRole('row', { name: new RegExp(escapeRegExp(label)) })
  }

  async selectResumeOption(label: string) {
    await this.resumeOptionRow(label).click()
  }

  useThisResumeButton(): Locator {
    return this.dialog().getByRole('button', { name: /^Use this resume$|^Saving…$/ })
  }

  clearSelectionButton(): Locator {
    return this.dialog().getByRole('button', { name: 'Clear selection' })
  }

  cancelButton(): Locator {
    return this.dialog().getByRole('button', { name: 'Cancel' })
  }
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
