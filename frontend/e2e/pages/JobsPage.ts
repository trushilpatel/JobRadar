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
 * - The always-open detail panel is `role="region"` named "Job details"; its
 *   title is an <h2>, and its status control is a `role="radiogroup"` named
 *   "Job status" containing `role="radio"` buttons labelled New/Applied/
 *   Interviewing/Offer/Rejected (jobStatusConfig.ts labels).
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
}
