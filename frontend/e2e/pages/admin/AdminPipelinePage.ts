import type { Page, Locator } from '@playwright/test'

export type PipelineWorker = 'filter' | 'enricher'
export type PipelineTier = '1m' | '10m' | '1h' | 'dlq'

const WORKER_LABELS: Record<PipelineWorker, string> = {
  filter: 'Filter retries',
  enricher: 'Enricher retries',
}

const TIER_LABELS: Record<PipelineTier, string> = {
  '1m': '1 min',
  '10m': '10 min',
  '1h': '1 hour',
  dlq: 'DLQ',
}

/**
 * Page Object for the admin Pipeline page
 * (frontend/src/pages/admin/AdminPipelinePage.tsx).
 *
 * Neither the retry-tier breakdown nor the ATS adapter table has a
 * data-testid — locators walk the real DOM structure from
 * `RetryBreakdown` (outer `div.rounded-lg` containing the exact worker
 * label, e.g. "Filter retries") and `AtsTable` (a plain `<table>`).
 *
 * Each tier row renders `<span>{tierLabel}</span><span>{count}</span><button>`
 * as three flex children of one row `div` — the row is located by finding
 * the exact-text tier-label span and walking up to its parent.
 */
export class AdminPipelinePage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto('/admin/pipeline')
  }

  pageHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Pipeline', exact: true })
  }

  /** The outer container div for one worker's retry breakdown card. */
  breakdownContainer(worker: PipelineWorker): Locator {
    return this.page
      .locator('div.rounded-lg')
      .filter({ has: this.page.getByText(WORKER_LABELS[worker], { exact: true }) })
  }

  /** The row div (tier label + count + Retry button) for one worker/tier pair. */
  tierRow(worker: PipelineWorker, tier: PipelineTier): Locator {
    return this.breakdownContainer(worker)
      .getByText(TIER_LABELS[tier], { exact: true })
      .locator('xpath=..')
  }

  tierCount(worker: PipelineWorker, tier: PipelineTier): Locator {
    // Row children: [0] tier label span, [1] count span, [2] Retry button.
    return this.tierRow(worker, tier).locator('span').nth(1)
  }

  tierRetryButton(worker: PipelineWorker, tier: PipelineTier): Locator {
    return this.tierRow(worker, tier).getByRole('button')
  }

  // ── ATS adapter table (read-only) ───────────────────────────────────────

  atsTable(): Locator {
    return this.page.getByRole('table')
  }

  atsRows(): Locator {
    return this.atsTable().locator('tbody tr')
  }
}
