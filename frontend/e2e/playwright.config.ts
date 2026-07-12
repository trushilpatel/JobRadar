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
      // Several specs here (jobs.spec.ts, apply-queue.spec.ts) act on the
      // SAME small set of live Tracking/Apply-Queue rows in the shared dev
      // dataset — e.g. both can end up targeting "whichever job happens to
      // be first" in their respective views. Running spec FILES within this
      // project in parallel let one test's dismiss/status-change race
      // another file's read of that identical row (confirmed: jobs.spec.ts's
      // dismiss/undo test timed out waiting for a row that apply-queue.spec.ts
      // was concurrently mutating). jobs.spec.ts already serializes its own
      // tests internally; this serializes across files too, since the
      // underlying data is effectively one more shared, non-disposable
      // resource (like the TEST_ROLE_USER account itself).
      fullyParallel: false,
    },
    {
      name: 'chromium-admin',
      testDir: './specs/admin',
      use: { ...devices['Desktop Chrome'], storageState: path.join(dirname, '.auth/admin.json') },
      dependencies: ['setup'],
    },
  ],
})
