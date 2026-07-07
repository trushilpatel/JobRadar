import type { Page } from '@playwright/test'

/** Page Object for the Account page (frontend/src/pages/AccountPage.tsx). */
export class AccountPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/account')
  }

  displayNameInput() {
    return this.page.getByLabel('Display name')
  }

  async readDisplayName(): Promise<string> {
    return this.displayNameInput().inputValue()
  }

  async setDisplayName(value: string) {
    const input = this.displayNameInput()
    await input.fill('')
    await input.fill(value)
  }

  saveButton() {
    return this.page.getByRole('button', { name: /^Save$/ })
  }

  async save() {
    await this.saveButton().click()
  }

  deleteAccountButton() {
    return this.page.getByRole('button', { name: 'Delete account' })
  }

  async openDeleteDialog() {
    await this.deleteAccountButton().click()
  }

  deleteForeverButton() {
    return this.page.getByRole('button', { name: 'Delete forever' })
  }

  async confirmDelete() {
    await this.deleteForeverButton().click()
  }
}
