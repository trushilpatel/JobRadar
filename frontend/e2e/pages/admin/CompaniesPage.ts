import type { Page, Locator } from '@playwright/test'

/**
 * Page Object for the admin Companies page
 * (frontend/src/pages/CompaniesPage.tsx + frontend/src/features/companies).
 */
export class CompaniesPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/admin/companies')
  }

  // ── Filter bar ─────────────────────────────────────────────────────────

  searchInput(): Locator {
    return this.page.getByPlaceholder('Search companies…')
  }

  async searchFor(term: string) {
    await this.searchInput().fill(term)
  }

  addCompanyButton(): Locator {
    return this.page.getByRole('button', { name: 'Add Company' })
  }

  async openAddCompanyModal() {
    await this.addCompanyButton().click()
  }

  // ── Add/Edit Company modal ────────────────────────────────────────────

  addCompanyDialog(): Locator {
    return this.page.getByRole('dialog', { name: 'Add Company' })
  }

  adapterSelect(): Locator {
    return this.addCompanyDialog().getByRole('combobox')
  }

  async selectAdapter(adapterName: string) {
    await this.adapterSelect().selectOption(adapterName)
  }

  identifierInput(): Locator {
    return this.addCompanyDialog().getByPlaceholder('stripe', { exact: true })
  }

  nameInput(): Locator {
    return this.addCompanyDialog().getByPlaceholder('Stripe', { exact: true })
  }

  startScanningSwitch(): Locator {
    return this.addCompanyDialog().getByRole('switch')
  }

  submitAddCompanyButton(): Locator {
    return this.addCompanyDialog().getByRole('button', { name: 'Add Company' })
  }

  /** Global (non-field) error message surfaced inside the Add Company dialog on submit failure. */
  addCompanyGlobalError(): Locator {
    return this.addCompanyDialog().locator('p.text-red-400')
  }

  /**
   * Fills and submits the Add Company form for a fixed-URL adapter (one whose
   * ATS entry has an empty base_url, e.g. "google") — no careers URL field is
   * required. Turns off "Start scanning immediately" so the disposable test
   * company never triggers a real crawl.
   *
   * Retries the submit once if the dialog is still open after the first
   * click (a transient backend error surfaced as a global error message
   * rather than the dialog closing) — the form's filled-in values are
   * untouched by a failed submit, so a second click is a safe retry for a
   * create call that never applied server-side. If the second attempt also
   * fails, the caller's own assertion on the created row surfaces the
   * (now-persistent, not transient) failure honestly.
   */
  async addFixedUrlCompany(opts: { adapter: string; id: string; name: string }) {
    await this.openAddCompanyModal()
    await this.selectAdapter(opts.adapter)
    await this.identifierInput().fill(opts.id)
    await this.nameInput().fill(opts.name)
    const scanSwitch = this.startScanningSwitch()
    if ((await scanSwitch.getAttribute('aria-checked')) === 'true') {
      await scanSwitch.click()
    }
    await this.submitAddCompanyButton().click()

    const closed = await this.addCompanyDialog()
      .waitFor({ state: 'hidden', timeout: 5_000 })
      .then(() => true)
      .catch(() => false)

    if (!closed) {
      await this.submitAddCompanyButton().click()
    }
  }

  // ── Table ──────────────────────────────────────────────────────────────

  row(name: string): Locator {
    return this.page.getByRole('row', { name: new RegExp(name) })
  }

  async isRowVisible(name: string): Promise<boolean> {
    return this.row(name).isVisible()
  }

  deleteButton(name: string): Locator {
    return this.row(name).getByRole('button', { name: `Delete ${name}` })
  }

  async openDeleteDialogFor(name: string) {
    await this.deleteButton(name).click()
  }

  // ── Delete confirmation dialog (rendered directly on CompaniesPage) ───

  deleteDialog(): Locator {
    return this.page.getByRole('alertdialog', { name: 'Delete Company' })
  }

  confirmDeleteButton(): Locator {
    return this.deleteDialog().getByRole('button', { name: 'Delete' })
  }

  async confirmDelete() {
    await this.confirmDeleteButton().click()
  }
}
