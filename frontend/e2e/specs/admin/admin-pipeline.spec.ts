import { test, expect } from '@playwright/test'
import {
  AdminPipelinePage,
  type PipelineTier,
  type PipelineWorker,
} from '../../pages/admin/AdminPipelinePage'

const WORKERS: PipelineWorker[] = ['filter', 'enricher']
const TIERS: PipelineTier[] = ['1m', '10m', '1h', 'dlq']

test.describe('Admin Pipeline page', () => {
  test('retry-tier controls match real counts, and the ATS adapter table renders', async ({
    page,
  }) => {
    const pipelinePage = new AdminPipelinePage(page)
    await pipelinePage.goto()

    await expect(pipelinePage.pageHeading()).toBeVisible()
    // Wait for the pipeline-health panel to finish loading (skeleton -> data)
    // before reading any tier row.
    await expect(pipelinePage.tierRow('filter', '1m')).toBeVisible({ timeout: 15_000 })

    // ── Step 1: every worker/tier's Retry button enabled-state must match
    // the real logic in RetryBreakdown: `disabled={counts[key] === 0 || isPending}`.
    // We read the on-screen count and assert the button's enabled state
    // against that same rule.
    let zeroCountTier: { worker: PipelineWorker; tier: PipelineTier } | null = null

    for (const worker of WORKERS) {
      for (const tier of TIERS) {
        const countLocator = pipelinePage.tierCount(worker, tier)
        const button = pipelinePage.tierRetryButton(worker, tier)

        await expect(countLocator).toBeVisible()
        await expect(button).toBeVisible()

        const rawCount = (await countLocator.textContent())?.trim() ?? ''
        const count = Number(rawCount)
        expect(
          Number.isNaN(count),
          `tier count for ${worker}/${tier} was not numeric: "${rawCount}"`
        ).toBe(false)

        if (count === 0) {
          await expect(button, `${worker}/${tier} shows 0 and must be disabled`).toBeDisabled()
          if (!zeroCountTier) zeroCountTier = { worker, tier }
        } else {
          await expect(button, `${worker}/${tier} shows ${count} and must be enabled`).toBeEnabled()
        }
      }
    }

    // ── Step 3: the ATS adapter-config table renders read-only with rows.
    await expect(pipelinePage.atsTable()).toBeVisible()
    const atsRowCount = await pipelinePage.atsRows().count()
    expect(atsRowCount).toBeGreaterThan(0)

    // ── Step 2: only exercise the click path on a genuinely zero-count tier.
    if (zeroCountTier) {
      const { worker, tier } = zeroCountTier
      const button = pipelinePage.tierRetryButton(worker, tier)
      const countLocator = pipelinePage.tierCount(worker, tier)

      let retryRequestFired = false
      page.on('request', (req) => {
        if (req.method() === 'POST' && req.url().includes('/admin/pipeline/retry')) {
          retryRequestFired = true
        }
      })

      // The button is disabled, so a real user click can never dispatch;
      // force bypasses Playwright's actionability wait (which would
      // otherwise time out waiting for "enabled") but the browser still
      // will not fire a click on a disabled <button>, so this is the safe
      // no-op the assignment calls for.
      await button.click({ force: true })
      await page.waitForTimeout(500)

      expect(
        retryRequestFired,
        'clicking a disabled Retry button must never call the retry API'
      ).toBe(false)
      await expect(button).toBeDisabled()
      await expect(countLocator).toHaveText('0')
    } else {
      test.info().annotations.push({
        type: 'coverage-gap',
        description:
          'Every visible retry tier (filter and enricher, all 4 tiers) showed a nonzero ' +
          'retryable count during this run, so the zero-count Retry-button click path was ' +
          'not exercised. Per assignment scope, no nonzero-count tier was clicked.',
      })
    }
  })
})
