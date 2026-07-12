import type { Page, Locator } from '@playwright/test'

/**
 * Page Object for the admin Audit Log page
 * (frontend/src/pages/admin/AdminAuditPage.tsx).
 *
 * Entirely read-only page — no mutations exist here. Filters debounce their
 * query 300ms (see `useDebouncedValue` in the component) before refetching.
 */
export class AdminAuditPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/admin/audit')
  }

  // ── Filter bar ─────────────────────────────────────────────────────────

  actorFilterInput(): Locator {
    return this.page.getByPlaceholder('Actor user ID or email…')
  }

  actionFilterInput(): Locator {
    return this.page.getByPlaceholder('Action (e.g. user.delete)…')
  }

  async filterByActor(value: string) {
    await this.actorFilterInput().fill(value)
  }

  async filterByAction(value: string) {
    await this.actionFilterInput().fill(value)
  }

  entriesCountLabel(): Locator {
    return this.page.getByText(/^\d+ entries$/)
  }

  // ── Table / empty state ───────────────────────────────────────────────

  table(): Locator {
    return this.page.locator('table')
  }

  rows(): Locator {
    return this.table().locator('tbody tr')
  }

  emptyState(): Locator {
    return this.page.getByText('No entries found.')
  }

  loadingState(): Locator {
    return this.page.getByText('Loading…')
  }

  // ── Pagination ─────────────────────────────────────────────────────────

  nextPageButton(): Locator {
    return this.page.getByRole('button', { name: 'Next page' })
  }

  prevPageButton(): Locator {
    return this.page.getByRole('button', { name: 'Previous page' })
  }

  /** The `aria-label="Page N"` text node rendered by the shared Pagination component. */
  pageIndicator(): Locator {
    return this.page.locator('p[aria-live="polite"]')
  }

  async isNextPageEnabled(): Promise<boolean> {
    return this.nextPageButton().isEnabled()
  }
}
