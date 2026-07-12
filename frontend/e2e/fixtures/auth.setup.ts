import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test as setup, expect } from '@playwright/test'

const authDir = path.join(path.dirname(fileURLToPath(import.meta.url)), '../.auth')

// Logs in via Clerk's real embedded sign-in UI (frontend/src/features/auth/components/SignInPage.tsx)
// — no Clerk testing-token bypass. Both test accounts use Clerk's official
// `+clerk_test@` convention: sign-in always challenges a "new device" email
// code, which resolves to the fixed OTP below rather than a real email.
// Credentials live in Doppler (dev config) — never hardcode them here.
async function signIn(
  page: import('@playwright/test').Page,
  email: string,
  password: string,
  otp: string
) {
  await page.goto('/login')
  await page.locator('input[name="identifier"]').fill(email)
  await page.locator('input[name="password"]').fill(password)
  await page.getByRole('button', { name: 'Continue', exact: true }).click()

  // New-device verification screen — one input captures the 6-digit code.
  await page.waitForTimeout(1000)
  await page.keyboard.type(otp, { delay: 50 })

  await expect(page).toHaveURL(/\/jobs/, { timeout: 15_000 })
}

function requireEnv(name: string): string {
  const value = process.env[name]
  if (!value) {
    throw new Error(`Missing ${name} — run via "npm run test:e2e" (wraps doppler run --config dev)`)
  }
  return value
}

setup('authenticate as user', async ({ page }) => {
  await signIn(
    page,
    requireEnv('TEST_ROLE_USER_EMAIL'),
    requireEnv('TEST_ROLE_USER_PASSWORD'),
    requireEnv('TEST_ROLE_USER_OTP')
  )
  await page.context().storageState({ path: path.join(authDir, 'user.json') })
})

setup('authenticate as admin', async ({ page }) => {
  await signIn(
    page,
    requireEnv('TEST_ROLE_ADMIN_EMAIL'),
    requireEnv('TEST_ROLE_ADMIN_PASSWORD'),
    requireEnv('TEST_ROLE_ADMIN_OTP')
  )
  await page.context().storageState({ path: path.join(authDir, 'admin.json') })
})
