import type { Page, Locator } from '@playwright/test'

/**
 * Page Object for the admin Users page
 * (frontend/src/pages/admin/AdminUsersPage.tsx).
 *
 * Note: the suspend/reinstate and delete row actions have no aria-label —
 * they rely on the native `title` attribute for a tooltip. Locate them with
 * `getByTitle(...)`, not `getByRole('button', { name })`.
 */
export class AdminUsersPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/admin/users')
  }

  // ── Filter bar ─────────────────────────────────────────────────────────

  emailFilterInput(): Locator {
    return this.page.getByPlaceholder('Filter by email…')
  }

  async searchByEmail(email: string) {
    await this.emailFilterInput().fill(email)
  }

  // ── Table row ──────────────────────────────────────────────────────────

  /** Row containing the given email — matched by literal text, not regex,
   * so `+` / `.` in disposable-user emails are never misinterpreted. */
  row(email: string): Locator {
    return this.page.getByRole('row').filter({ hasText: email })
  }

  async isRowVisible(email: string): Promise<boolean> {
    return this.row(email).isVisible()
  }

  statusBadge(email: string): Locator {
    // StatusBadge renders lowercase "active" / "suspended" text in a span.
    return this.row(email).getByText(/^(active|suspended)$/)
  }

  async readStatus(email: string): Promise<string> {
    return (await this.statusBadge(email).textContent())?.trim() ?? ''
  }

  suspendReinstateButton(email: string): Locator {
    return this.row(email).locator('button[title="Suspend"], button[title="Reinstate"]')
  }

  async toggleSuspend(email: string) {
    await this.suspendReinstateButton(email).click()
  }

  deleteButton(email: string): Locator {
    return this.row(email).getByTitle('Delete user')
  }

  async openDeleteDialogFor(email: string) {
    await this.deleteButton(email).click()
  }

  // ── Delete confirmation dialog ────────────────────────────────────────

  deleteDialog(email: string): Locator {
    return this.page.getByRole('dialog', { name: `Delete ${email}?` })
  }

  confirmDeleteForeverButton(email: string): Locator {
    return this.deleteDialog(email).getByRole('button', { name: 'Delete forever' })
  }

  async confirmDelete(email: string) {
    await this.confirmDeleteForeverButton(email).click()
  }
}
