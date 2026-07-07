import type { Page, Locator } from '@playwright/test'

/**
 * Page Object for the Jobs feed (frontend/src/pages/JobsPage.tsx) and its
 * child features under frontend/src/features/jobs/.
 *
 * Locator notes:
 * - Table rows render as `<tr role="button" aria-label="{title} at {company},
 *   status {status}">` (JobsTable's `rowLabel` + `getRowProps`), so every row
 *   is reachable as an accessible button whose name always contains
 *   ", status ".
 * - The bookmark/save button is the first `<button>` in the row
 *   (jobs-columns.tsx "bookmark" column), aria-label
 *   `Save {title}` when unsaved or `Remove {title} from saved` when saved.
 * - The dismiss button is hover-revealed (opacity-0 until row hover/focus)
 *   but always present in the DOM; aria-label `Dismiss {title}`.
 * - The always-open detail panel is `role="region"` named "Job details"; its
 *   title is an <h2>, and its status control is a `role="radiogroup"` named
 *   "Job status" containing `role="radio"` buttons labelled New/Applied/
 *   Interviewing/Offer/Rejected (jobStatusConfig.ts labels).
 * - Dismissing a job fires TWO toasts (see useJobActions.ts + JobsPage.tsx
 *   handleDismiss) — a generic sonner `useUndoToast` AND the richer
 *   `DismissStripe` (data-testid="dismiss-stripe"). Both call the same
 *   `undoLast()`, but to avoid a strict-mode locator clash between two
 *   "Undo" buttons on screen at once, always click Undo scoped to the
 *   dismiss-stripe testid.
 */
export class JobsPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/jobs')
  }

  get sentinel(): Locator {
    return this.page.getByTestId('jobs-table-sentinel')
  }

  /** All job rows currently rendered in the table body. */
  rows(): Locator {
    return this.page.getByRole('button', { name: /, status \S+$/ })
  }

  async rowCount(): Promise<number> {
    return this.rows().count()
  }

  firstRow(): Locator {
    return this.rows().first()
  }

  /** Row whose accessible name contains this exact job title (case-sensitive). */
  rowByTitle(title: string): Locator {
    return this.rows().filter({ hasText: title })
  }

  async openRow(row: Locator) {
    await row.click()
  }

  /** Visible title text rendered in the row's "Role" column (first <td>'s span). */
  async titleOf(row: Locator): Promise<string> {
    return (await row.locator('td').nth(1).innerText()).trim()
  }

  /**
   * Company name for this row, read from the CompanyAvatar's
   * `role="img" aria-label="{name}"` in the "Company" column — avoids
   * `innerText()` picking up the avatar's own 2-letter initials text
   * (rendered adjacent to, but distinct from, the company name span).
   */
  async companyOf(row: Locator): Promise<string> {
    const label = await row.locator('td').nth(2).getByRole('img').getAttribute('aria-label')
    return (label ?? '').trim()
  }

  /** The row's one-click save/bookmark toggle button, in either save state. */
  saveButton(row: Locator, title: string): Locator {
    return row
      .getByRole('button', { name: `Save ${title}`, exact: true })
      .or(row.getByRole('button', { name: `Remove ${title} from saved`, exact: true }))
  }

  /** The row's hover-revealed dismiss ("X") button. */
  dismissButton(row: Locator, title: string): Locator {
    return row.getByRole('button', { name: `Dismiss ${title}`, exact: true })
  }

  // ── Job detail panel (features/jobs/components/JobDetailPanel.tsx) ────────

  get detailPanel(): Locator {
    return this.page.getByRole('region', { name: 'Job details' })
  }

  get detailTitle(): Locator {
    return this.detailPanel.locator('h2')
  }

  get detailStatusGroup(): Locator {
    return this.detailPanel.getByRole('radiogroup', { name: 'Job status' })
  }

  /** Status segmented-control radio button, e.g. "New" | "Applied" | "Interviewing". */
  detailStatusRadio(label: string): Locator {
    return this.detailStatusGroup.getByRole('radio', { name: label, exact: true })
  }

  closeDetailButton(): Locator {
    return this.page.getByRole('button', { name: 'Close job details' })
  }

  // ── Dismiss + undo (DismissStripe) ─────────────────────────────────────────

  get dismissStripe(): Locator {
    return this.page.getByTestId('dismiss-stripe')
  }

  get dismissStripeUndoButton(): Locator {
    return this.dismissStripe.getByRole('button', { name: 'Undo', exact: true })
  }
}
