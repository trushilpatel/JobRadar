import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

const dirname = path.dirname(fileURLToPath(import.meta.url))

// Run via `npm run test:e2e` (wraps with `doppler run --` so TEST_ROLE_*
// credentials and CLERK_SECRET_KEY are present in process.env). Assumes the
// local dev stack (`make dev`) is already running on :5173 / :8080 — this
// suite does not manage the dev server lifecycle itself.
export default defineConfig({
  testDir: '.',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  outputDir: path.join(dirname, '../test-results'),
  reporter: [['html', { outputFolder: path.join(dirname, '../playwright-report'), open: 'never' }]],
  timeout: 30_000,
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'setup',
      testDir: './fixtures',
      testMatch: /.*\.setup\.ts/,
    },
    {
      name: 'chromium-user',
      testDir: './specs/user',
      use: { ...devices['Desktop Chrome'], storageState: path.join(dirname, '.auth/user.json') },
      dependencies: ['setup'],
    },
    {
      name: 'chromium-admin',
      testDir: './specs/admin',
      use: { ...devices['Desktop Chrome'], storageState: path.join(dirname, '.auth/admin.json') },
      dependencies: ['setup'],
    },
  ],
})
